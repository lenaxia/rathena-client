-- Test account + one ready-to-play character for the live integration test.
-- Account 2000001 / testbot / testpass (GM group 99).
-- Character TestBot, slot 0, Novice in Prontera.

INSERT INTO `login`
    (`account_id`, `userid`, `user_pass`, `sex`, `email`, `group_id`, `birthdate`)
VALUES
    (2000001, 'testbot', 'testpass', 'M', 'testbot@example.invalid', 99, '1990-01-01');

INSERT INTO `char`
    (`account_id`, `char_num`, `name`, `class`, `base_level`, `job_level`,
     `zeny`, `str`, `agi`, `vit`, `int`, `dex`, `luk`,
     `max_hp`, `hp`, `max_sp`, `sp`, `status_point`, `skill_point`,
     `hair`, `last_map`, `last_x`, `last_y`, `save_map`, `save_x`, `save_y`)
VALUES
    (2000001, 0, 'TestBot', 0, 45, 20,
     100000, 40, 40, 40, 40, 40, 40,
     3000, 3000, 200, 200, 100, 40,
     1, 'prontera', 150, 150, 'prontera', 150, 150);
