package gen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"

	"github.com/lenaxia/rathena-client/internal/codegen/preprocess"
	"github.com/lenaxia/rathena-client/internal/codegen/semantics"
)

// GenerateRegisterFile generates the Go source for pkg/encode/register.go.
//
// The file contains an init() function that calls session.RegisterSendEncoder
// for every send-direction semantic action that has a corresponding Encode*
// function in pkg/encode/. Only actions whose Encode<Name> function actually
// exists in encodeDir are included.
//
// Pass encodeDir="" to include all actions with send-direction implementations
// regardless of whether a function exists (used in unit tests with mock data).
//
// The TotalSize from the VersionTable determines whether the wrapper uses
// the fixed-array b[:] form or the direct []byte form. Nil layouts are skipped
// (treated as "not present for size computation"), mirroring encode.go behavior.
// An action where all layouts are nil is treated as variable-length.
func GenerateRegisterFile(db *semantics.DB, vt preprocess.VersionTable) (string, error) {
	return generateRegisterFileInner(db, vt, "")
}

// GenerateRegisterFileWithDir is like GenerateRegisterFile but scans encodeDir
// for existing Encode* functions to skip actions that have no encode file.
func GenerateRegisterFileWithDir(db *semantics.DB, vt preprocess.VersionTable, encodeDir string) (string, error) {
	return generateRegisterFileInner(db, vt, encodeDir)
}

func generateRegisterFileInner(db *semantics.DB, vt preprocess.VersionTable, encodeDir string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("nil DB")
	}

	// Build set of existing Encode* function names and their return types.
	// fixedReturnEncoders holds function names that return a fixed [N]byte array.
	existingEncoders := map[string]bool{}
	fixedReturnEncoders := map[string]bool{}
	if encodeDir != "" {
		entries, err := os.ReadDir(encodeDir)
		if err == nil {
			fset := token.NewFileSet()
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
					continue
				}
				// Skip the register.go file itself.
				if entry.Name() == "register.go" {
					continue
				}
				path := encodeDir + "/" + entry.Name()
				f, err := parser.ParseFile(fset, path, nil, 0)
				if err != nil {
					continue
				}
				for _, decl := range f.Decls {
					fd, ok := decl.(*ast.FuncDecl)
					if !ok {
						continue
					}
					if !strings.HasPrefix(fd.Name.Name, "Encode") {
						continue
					}
					existingEncoders[fd.Name.Name] = true
					// Check if the return type is a fixed [N]byte array.
					if fd.Type.Results != nil && len(fd.Type.Results.List) == 1 {
						ret := fd.Type.Results.List[0].Type
						if arr, ok := ret.(*ast.ArrayType); ok && arr.Len != nil {
							fixedReturnEncoders[fd.Name.Name] = true
						}
					}
				}
			}
		}
	}

	names := make([]string, 0, len(db.Actions))
	for name := range db.Actions {
		names = append(names, name)
	}
	sort.Strings(names)

	type registerEntry struct {
		actionConst string
		structName  string
		isFixed     bool
	}

	var entries []registerEntry
	var skipped []string

	for _, name := range names {
		action := db.Actions[name]
		goIdent := actionNameToGoIdent(name)

		// Collect send-direction implementations only.
		var sendImpls []semantics.Implementation
		for _, impl := range action.Implementations {
			if isSendStruct(impl.StructName) {
				sendImpls = append(sendImpls, impl)
			}
		}
		if len(sendImpls) == 0 {
			skipped = append(skipped, name)
			continue
		}

		// If encodeDir is provided, verify the Encode* function actually exists.
		// This handles the case where the encode codegen skips an action (e.g.
		// because its struct layout is unknown), but the action has a send-direction
		// entry in the DB.
		encodeFuncName := fmt.Sprintf("Encode%s", goIdent)
		if encodeDir != "" && !existingEncoders[encodeFuncName] {
			skipped = append(skipped, name)
			continue
		}

		// Determine whether the Encode* function returns a fixed [N]byte or []byte.
		// This mirrors the commonSize aggregation in generateEncodeDispatcher
		// (encode.go:181-198): nil layouts are skipped (continue), not treated as
		// variable-length — only TotalSize <= 0 or size disagreement triggers []byte.
		// An action where all layouts are nil (e.g. public_chat / PACKET_CZ_REQUEST_CHAT
		// which has no struct in rAthena) results in commonSize == -1 → isFixed = false
		// (variable path), which is correct since the hand-written encode returns []byte.
		commonSize := -1
		for _, impl := range sendImpls {
			layout := resolveLayout(impl.StructName, impl.PacketverMin, vt)
			if layout == nil {
				continue
			}
			if layout.TotalSize <= 0 {
				commonSize = 0
				break
			}
			if commonSize == -1 {
				commonSize = layout.TotalSize
			} else if commonSize != layout.TotalSize {
				commonSize = 0
				break
			}
		}

		isFixed := commonSize > 0
		// When we have actual function signatures from scanning the encode dir,
		// use those to determine fixed vs variable. This handles hand-written
		// encode functions (e.g. EncodeDealFinalize returns [2]byte despite having
		// no struct in the VersionTable).
		if encodeDir != "" {
			isFixed = fixedReturnEncoders[encodeFuncName]
		}

		entries = append(entries, registerEntry{
			actionConst: fmt.Sprintf("Action%s", goIdent),
			structName:  goIdent,
			isFixed:     isFixed,
		})
	}

	var sb strings.Builder
	sb.WriteString("// Code generated by internal/codegen. DO NOT EDIT.\n\n")
	sb.WriteString("package encode\n\n")
	sb.WriteString("import (\n")
	sb.WriteString("\t\"github.com/lenaxia/rathena-client/pkg/send\"\n")
	sb.WriteString("\t\"github.com/lenaxia/rathena-client/pkg/session\"\n")
	sb.WriteString(")\n\n")

	if len(skipped) > 0 {
		sb.WriteString("// The following actions have no send-direction implementations\n")
		sb.WriteString("// or no corresponding Encode* function, and are therefore absent\n")
		sb.WriteString("// from this init() function:\n")
		for _, name := range skipped {
			sb.WriteString(fmt.Sprintf("//   %s\n", name))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("func init() {\n")
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("\tsession.RegisterSendEncoder(session.%s,\n", e.actionConst))
		sb.WriteString(fmt.Sprintf("\t\tfunc(req interface{}, pv uint32) ([]byte, error) {\n"))
		sb.WriteString(fmt.Sprintf("\t\t\tr, ok := req.(send.%s)\n", e.structName))
		sb.WriteString(fmt.Sprintf("\t\t\tif !ok {\n"))
		sb.WriteString(fmt.Sprintf("\t\t\t\treturn nil, session.ErrWrongSendType\n"))
		sb.WriteString(fmt.Sprintf("\t\t\t}\n"))
		if e.isFixed {
			sb.WriteString(fmt.Sprintf("\t\t\tb := Encode%s(r, pv)\n", e.structName))
			sb.WriteString(fmt.Sprintf("\t\t\treturn b[:], nil\n"))
		} else {
			sb.WriteString(fmt.Sprintf("\t\t\treturn Encode%s(r, pv), nil\n", e.structName))
		}
		sb.WriteString(fmt.Sprintf("\t\t},\n"))
		sb.WriteString(fmt.Sprintf("\t)\n"))
	}
	sb.WriteString("}\n")

	return sb.String(), nil
}
