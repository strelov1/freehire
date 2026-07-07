# Telegram channels

Human-readable index of the public Telegram channels crawled by `cmd/tg-ingest`
(see `internal/telegram`). **The source of truth is [`sources/telegram.yml`](../sources/telegram.yml)** —
this document mirrors it for browsing; when the YAML changes, regenerate this list.

**88 channels** (12 `authored`, 76 `board`) as of the last update.

`kind` steers the extraction prompt:

- **`authored`** — editorial channel; one post may bundle several vacancies (0..N).
- **`board`** — job-board channel; one post is normally one vacancy.

Aggregator-bot channels that repost their own job sites (e.g. `remoteyeah`) are
deliberately excluded — they duplicate the ATS boards `sources/*.yml` already cover.

## Curated tier-1 (RU / IT)

Verified 2026-06-12: public `t.me/s` preview enabled and a post within the last 30 days.

### Authored / curated

| Channel | kind |
|---|---|
| [hrlunapark](https://t.me/hrlunapark) | authored |
| [normrabota](https://t.me/normrabota) | authored |
| [budujobs](https://t.me/budujobs) | authored |
| [streltsova_anastasiya](https://t.me/streltsova_anastasiya) | authored |

### General IT boards

| Channel | kind |
|---|---|
| [it_vakansii_jobs](https://t.me/it_vakansii_jobs) | board |
| [geekjobs](https://t.me/geekjobs) | board |
| [vakansii_it](https://t.me/vakansii_it) | board |
| [zrabota](https://t.me/zrabota) | board |
| [habr_career](https://t.me/habr_career) | board |
| [huntmejob](https://t.me/huntmejob) | board |
| [newdirections](https://t.me/newdirections) | board |

### Junior / internships

| Channel | kind |
|---|---|
| [jobforjunior](https://t.me/jobforjunior) | board |
| [young_june](https://t.me/young_june) | board |
| [juniors_rabota_jobs](https://t.me/juniors_rabota_jobs) | board |
| [jobs_juniors_remote](https://t.me/jobs_juniors_remote) | board |
| [job_it_junior](https://t.me/job_it_junior) | board |
| [remotejun](https://t.me/remotejun) | board |
| [juno_jobs](https://t.me/juno_jobs) | board |
| [young_intern](https://t.me/young_intern) | board |
| [it_interns](https://t.me/it_interns) | board |
| [refer_me_it](https://t.me/refer_me_it) | board |

### Role-specific boards

| Channel | kind |
|---|---|
| [product_jobs](https://t.me/product_jobs) | board |
| [productjobgo](https://t.me/productjobgo) | board |
| [hireproproduct](https://t.me/hireproproduct) | board |
| [forproducts](https://t.me/forproducts) | board |
| [foranalysts](https://t.me/foranalysts) | board |
| [serious_tester](https://t.me/serious_tester) | board |
| [forallqa](https://t.me/forallqa) | board |
| [job_python](https://t.me/job_python) | board |

### Remote / relocation

| Channel | kind |
|---|---|
| [Remoteit](https://t.me/Remoteit) | board |
| [young_relocate](https://t.me/young_relocate) | board |
| [remote_jobs_relocate](https://t.me/remote_jobs_relocate) | board |

### Corporate

| Channel | kind |
|---|---|
| [avito_career](https://t.me/avito_career) | board |
| [mtsbankcareer](https://t.me/mtsbankcareer) | board |

### Web3

| Channel | kind |
|---|---|
| [job_web3](https://t.me/job_web3) | board |
| [crypto_vacancy_web3](https://t.me/crypto_vacancy_web3) | board |
| [careers_crypto](https://t.me/careers_crypto) | board |

## Discovered via telagon (2026-06-16)

Top channels by ATS-link volume; all verified to have a live public `t.me/s`
preview at discovery time. Mostly EN India/global plus a few RU/PT/AR/FA/Web3
aggregators — geography the curated tier above did not cover.

### Global / India tech job boards

| Channel | kind |
|---|---|
| [dot_aware](https://t.me/dot_aware) | board |
| [gocareers](https://t.me/gocareers) | board |
| [getjobss](https://t.me/getjobss) | board |
| [jobs_and_internships_updates](https://t.me/jobs_and_internships_updates) | board |
| [jobsandinternshipsupdates](https://t.me/jobsandinternshipsupdates) | board |
| [off_campus_jobs_and_internships](https://t.me/off_campus_jobs_and_internships) | board |
| [offcampus_phodenge](https://t.me/offcampus_phodenge) | board |
| [freshercareersdotin](https://t.me/freshercareersdotin) | board |
| [OceanOfJobs](https://t.me/OceanOfJobs) | board |
| [JobsPur](https://t.me/JobsPur) | board |
| [jobvila](https://t.me/jobvila) | board |
| [hiringdaily](https://t.me/hiringdaily) | board |
| [TorchBearerr](https://t.me/TorchBearerr) | board |
| [jobsstation_official](https://t.me/jobsstation_official) | board |
| [tamilanjobupdates](https://t.me/tamilanjobupdates) | board |

### India personal-brand / editorial

Posts may bundle several vacancies.

| Channel | kind |
|---|---|
| [arunchauhanofficial](https://t.me/arunchauhanofficial) | authored |
| [jobwithmayra](https://t.me/jobwithmayra) | authored |
| [goyalarsh](https://t.me/goyalarsh) | authored |
| [vijaykushal](https://t.me/vijaykushal) | authored |
| [PrepTrain](https://t.me/PrepTrain) | authored |
| [cs_algo](https://t.me/cs_algo) | authored |

### Data / analytics niche

| Channel | kind |
|---|---|
| [dataanalyticsbuddy](https://t.me/dataanalyticsbuddy) | board |
| [cv_2essence](https://t.me/cv_2essence) | board |

### Nigeria

| Channel | kind |
|---|---|
| [tohire_ng](https://t.me/tohire_ng) | board |
| [jobnetworkng](https://t.me/jobnetworkng) | board |
| [dejob_global](https://t.me/dejob_global) | board |
| [DeJob_official](https://t.me/DeJob_official) | board |

### STEM / students

| Channel | kind |
|---|---|
| [STEMJobsCR](https://t.me/STEMJobsCR) | board |

### Other languages (ar / fa / pt / ru)

| Channel | kind |
|---|---|
| [amalw3amal1](https://t.me/amalw3amal1) | board |
| [seekingyourjobs](https://t.me/seekingyourjobs) | board |
| [cafeinavagas](https://t.me/cafeinavagas) | board |
| [youritjob](https://t.me/youritjob) | board |

### Web3

| Channel | kind |
|---|---|
| [worklinketh](https://t.me/worklinketh) | board |
| [talentatweb3](https://t.me/talentatweb3) | board |

## Added 2026-06-19 (user-supplied list)

Verified to have a live public `t.me/s` preview at add time. Skews RU and toward
executive / top-management / marketing roles rather than pure IT.

### Executive / top-management (RU)

| Channel | kind |
|---|---|
| [work_for_top](https://t.me/work_for_top) | board |
| [workfortop](https://t.me/workfortop) | board |
| [forchiefs](https://t.me/forchiefs) | board |
| [xCareers](https://t.me/xCareers) | board |
| [middle_top_vacancies](https://t.me/middle_top_vacancies) | board |
| [cgrowthcareer](https://t.me/cgrowthcareer) | authored |

### General RU job boards

| Channel | kind |
|---|---|
| [vacanciesbest](https://t.me/vacanciesbest) | board |
| [morejobs](https://t.me/morejobs) | board |
| [moskovskayarabota](https://t.me/moskovskayarabota) | authored |
| [digital_hr](https://t.me/digital_hr) | board |
| [huggabletalents](https://t.me/huggabletalents) | board |

### Remote / relocation

| Channel | kind |
|---|---|
| [evacuatejobs](https://t.me/evacuatejobs) | board |
| [remotegeekjob](https://t.me/remotegeekjob) | board |
| [zarubezhom_jobs](https://t.me/zarubezhom_jobs) | board |

### Marketing niche

| Channel | kind |
|---|---|
| [marketing_jobs](https://t.me/marketing_jobs) | board |
| [forallmarketing](https://t.me/forallmarketing) | board |
| [wantapply_marketing](https://t.me/wantapply_marketing) | board |
