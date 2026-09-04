-- Telegram channels move from sources/telegram.yml into the database, the last thing
-- that file held. The board catalog moved in 0123; this closes the same loop for the
-- other list cmd/tg-ingest and cmd/tg-extract read.
--
-- kind steers the extraction prompt: 'authored' is an editorial channel whose post may
-- bundle several vacancies (0..N), 'board' a job-board channel where one post is one
-- vacancy. The check constraint holds what internal/ingest/telegram's Config.Validate
-- used to enforce over YAML.
--
-- The identity is case-insensitive, matching t.me username matching in the package:
-- "hrlunapark" and "HRLunapark" name the same channel and would otherwise both crawl,
-- writing the same posts twice under different external_ids.
--
-- A retired channel flips active=false rather than being deleted, the same shape boards
-- uses: telegram_posts rows reference the channel by name and outlive the decision to
-- stop crawling it.
CREATE TABLE telegram_channels (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    channel    text NOT NULL,
    kind       text NOT NULL CHECK (kind IN ('authored', 'board')),
    active     boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX telegram_channels_name_key ON telegram_channels (lower(channel));

-- The curated list this table replaces, verbatim from the file it retires. It is
-- configuration, not user data, and this migration is its only delivery — there is no
-- file left for a backfill worker to read. ON CONFLICT keeps a re-run inert.
INSERT INTO telegram_channels (channel, kind) VALUES
    ('hrlunapark', 'authored'),
    ('normrabota', 'authored'),
    ('budujobs', 'authored'),
    ('streltsova_anastasiya', 'authored'),
    ('it_vakansii_jobs', 'board'),
    ('vakansii_it', 'board'),
    ('zrabota', 'board'),
    ('huntmejob', 'board'),
    ('newdirections', 'board'),
    ('jobforjunior', 'board'),
    ('young_june', 'board'),
    ('juniors_rabota_jobs', 'board'),
    ('jobs_juniors_remote', 'board'),
    ('job_it_junior', 'board'),
    ('remotejun', 'board'),
    ('juno_jobs', 'board'),
    ('young_intern', 'board'),
    ('it_interns', 'board'),
    ('refer_me_it', 'board'),
    ('product_jobs', 'board'),
    ('productjobgo', 'board'),
    ('hireproproduct', 'board'),
    ('forproducts', 'board'),
    ('foranalysts', 'board'),
    ('serious_tester', 'board'),
    ('forallqa', 'board'),
    ('job_python', 'board'),
    ('Remoteit', 'board'),
    ('young_relocate', 'board'),
    ('remote_jobs_relocate', 'board'),
    ('avito_career', 'board'),
    ('mtsbankcareer', 'board'),
    ('job_web3', 'board'),
    ('crypto_vacancy_web3', 'board'),
    ('careers_crypto', 'board'),
    ('dot_aware', 'board'),
    ('gocareers', 'board'),
    ('jobs_and_internships_updates', 'board'),
    ('jobsandinternshipsupdates', 'board'),
    ('off_campus_jobs_and_internships', 'board'),
    ('offcampus_phodenge', 'board'),
    ('freshercareersdotin', 'board'),
    ('OceanOfJobs', 'board'),
    ('JobsPur', 'board'),
    ('jobvila', 'board'),
    ('hiringdaily', 'board'),
    ('TorchBearerr', 'board'),
    ('jobsstation_official', 'board'),
    ('tamilanjobupdates', 'board'),
    ('arunchauhanofficial', 'authored'),
    ('jobwithmayra', 'authored'),
    ('goyalarsh', 'authored'),
    ('vijaykushal', 'authored'),
    ('PrepTrain', 'authored'),
    ('cs_algo', 'authored'),
    ('dataanalyticsbuddy', 'board'),
    ('cv_2essence', 'board'),
    ('tohire_ng', 'board'),
    ('jobnetworkng', 'board'),
    ('dejob_global', 'board'),
    ('DeJob_official', 'board'),
    ('STEMJobsCR', 'board'),
    ('amalw3amal1', 'board'),
    ('seekingyourjobs', 'board'),
    ('cafeinavagas', 'board'),
    ('youritjob', 'board'),
    ('worklinketh', 'board'),
    ('talentatweb3', 'board'),
    ('work_for_top', 'board'),
    ('workfortop', 'board'),
    ('forchiefs', 'board'),
    ('xCareers', 'board'),
    ('middle_top_vacancies', 'board'),
    ('cgrowthcareer', 'authored'),
    ('vacanciesbest', 'board'),
    ('morejobs', 'board'),
    ('moskovskayarabota', 'authored'),
    ('digital_hr', 'board'),
    ('huggabletalents', 'board'),
    ('evacuatejobs', 'board'),
    ('zarubezhom_jobs', 'board'),
    ('marketing_jobs', 'board'),
    ('forallmarketing', 'board'),
    ('wantapply_marketing', 'board'),
    ('wantapply_managers', 'board'),
    ('wantapply_design', 'board'),
    ('wantapply_analytics', 'board'),
    ('wantapply_qa_jobs', 'board'),
    ('naymarnya', 'board'),
    ('halepnyirecruiting', 'board'),
    ('devops_dou', 'authored'),
    ('dou_qa', 'authored'),
    ('frontend_dou', 'authored'),
    ('gamedev_dou', 'authored'),
    ('junior_dou_ua', 'authored')
ON CONFLICT DO NOTHING;
