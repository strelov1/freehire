# 03 — What an application form asks beyond name, email and CV

Measured against production (`hire`) on **2026-09-09**. Read-only.

## Query A — look at the raw labels before choosing how to count them

Labels are stored verbatim by design (`internal/ingest/applyform/AGENTS.md`), so the first
job is to find out how badly one question fragments across spellings. A 2% sample, top 60
by frequency, was enough to answer that.

It fragments badly:

- résumé: `resume/cv` · `resume` · `cv` · `currículo/cv` · `lebenslauf` · `lettre de motivation`
- LinkedIn: `linkedin profile` · `linkedin url` · `linkedin`
- phone: `phone` · `phone number`
- referral: `how did you hear about this job?` · `how did you hear about us?` ·
  `how did you hear about this opportunity?`
- age gate: `are you at least 18 years of age?` · `are you 18 years of age or older?`
- salary: `what are your salary expectations?` · `what is your desired salary?`
- ethnicity: `how would you describe your racial/ethnic background?` and the same question
  again with ` (mark all that apply)` appended

A raw `GROUP BY` would report each of those as separate questions and understate every one
of them. A naive normalisation (case, punctuation, whitespace) merges the last pair and
none of the others.

## Grouping method chosen, and why

**Hand-grouped families, with every folded label listed.** It is the only honest option
here: the fragments differ in wording, not in formatting, so no mechanical rule reaches
them, and a rule that reached *some* of them would be worse than none — it would look
principled while still undercounting.

The families and their members are listed below in full. A reader can check every one
against the raw counts.

## Query B — full-corpus label frequencies

Run over all 647,795 forms, not a sample. `providers` counts how many of the five platforms
a label appears on — added deliberately, and it changed a conclusion (see below).

```sql
SELECT lower(trim(f ->> 'label')) AS label,
       count(*)                   AS n,
       count(DISTINCT provider)   AS providers
FROM apply_forms, jsonb_array_elements(payload -> 'fields') f
WHERE coalesce(f ->> 'label', '') <> ''
GROUP BY 1
HAVING count(*) >= 3000
ORDER BY n DESC
LIMIT 70;
```

`36086.119 ms (00:36.086)`. Raw output, unedited:

```
resume/cv|650461|3
email|625151|5
phone|550638|5
cover letter|536396|5
first name|345173|5
last name|344499|5
current location|243452|5
full name|208149|4
linkedin profile|197569|5
resume|153978|5
location|136851|5
current company|126184|5
preferred first name|121373|5
latitude|118203|1
longitude|118203|1
gender|113269|5
race|106774|4
disabilitystatus|104970|1
veteranstatus|104970|1
linkedin url|91649|5
cv|90584|5
website|84692|5
name|65052|5
address|62029|5
portfolio url|59543|5
github url|58455|5
photo|46323|3
twitter url|44907|3
experience|44679|4
education|43324|5
other website|40873|3
summary|39606|2
are you legally authorized to work in the united states?|36058|5
phone number|32043|5
headline|30527|2
how did you hear about this job?|28222|5
pronouns|26004|5
how did you hear about us?|22148|5
are you at least 18 years of age?|21367|5
linkedin|19730|5
what are your salary expectations?|15972|5
city|13858|5
do you identify as transgender?|13413|2
are you a veteran or active member of the united states armed forces?|12751|2
do you have a disability or chronic condition (physical, visual, auditory, cognitive, mental, emotional, or other) that substantially limits one or more of your major life activities, including mobility, communication (seeing, hearing, speaking), and learning?|11528|2
how did you hear about this opportunity?|11392|5
state|11284|5
what gender do you identify as?|10548|4
what is your age range?|10321|4
lettre de motivation|9762|4
currículo/cv|9556|1
do you currently reside in the united states?|9420|5
e-mail|9188|4
are you authorized to work in the united states?|9157|5
how would you describe your gender identity?|9002|2
are you 18 years of age or older?|8970|5
what is your desired salary?|8790|5
i identify my ethnicity asselect all that apply|8683|1
how would you describe your racial/ethnic background? (mark all that apply)|8430|1
will you now or in the future require sponsorship for employment visa status?|8413|5
how would you describe your gender identity? (mark all that apply)|8300|1
how would you describe your racial/ethnic background?|8106|1
zip code|7870|5
what is your notice period?|7841|5
which location are you applying for?|7814|5
lebenslauf|7752|5
will you now or in the future require visa sponsorship?|7587|5
how would you describe your sexual orientation? (mark all that apply)|7550|1
country|7507|5
preferred name|7390|5
```

## The `providers` column changed a conclusion

`latitude` and `longitude` appear 118,203 times each — and on **one** platform. So do
`disabilitystatus` and `veteranstatus`, at 104,970 each.

Without that column the honest-looking sentence would have been "application forms collect
your exact coordinates," which reads as an industry practice. It is one vendor's template.
The article says the second thing, not the first.

The contrast is what makes the column worth its width: `gender` (113,269) appears on all
five platforms and `race` (106,774) on four. Those are practices. Coordinates are a
product decision by one company.

## The families

Every member label is listed; sum each column to check the total.

**Excluded as the floor — on every form, so not a finding:** `resume/cv` 650,461 · `email`
625,151 · `phone` 550,638 · `cover letter` 536,396 · `first name` 345,173 · `last name`
344,499 · `full name` 208,149 · `resume` 153,978 · `preferred first name` 121,373 · `cv`
90,584 · `name` 65,052 · `phone number` 32,043 · `e-mail` 9,188 · `lebenslauf` 7,752 ·
`lettre de motivation` 9,762 · `currículo/cv` 9,556 · `preferred name` 7,390.

**Where you live** — `current location` 243,452 · `location` 136,851 · `address` 62,029 ·
`city` 13,858 · `state` 11,284 · `zip code` 7,870 · `country` 7,507. Plus, on one platform,
`latitude` + `longitude` at 118,203 each.

**Links to the rest of you** — `linkedin profile` 197,569 · `linkedin url` 91,649 ·
`linkedin` 19,730 (LinkedIn total **308,948**) · `website` 84,692 · `portfolio url` 59,543 ·
`github url` 58,455 · `twitter url` 44,907 · `other website` 40,873 · `photo` 46,323.

**Demographics** — `gender` 113,269 · `race` 106,774 · `disabilitystatus` 104,970 (1
platform) · `veteranstatus` 104,970 (1 platform) · `do you identify as transgender?` 13,413
· `are you a veteran or active member of the united states armed forces?` 12,751 · the
long-form disability question 11,528 · `what gender do you identify as?` 10,548 · `what is
your age range?` 10,321 · `how would you describe your gender identity?` 9,002 · `i
identify my ethnicity asselect all that apply` 8,683 · `how would you describe your
racial/ethnic background? (mark all that apply)` 8,430 · `how would you describe your
gender identity? (mark all that apply)` 8,300 · `how would you describe your racial/ethnic
background?` 8,106 · `how would you describe your sexual orientation? (mark all that
apply)` 7,550 · `pronouns` 26,004.

**Work authorization** — `are you legally authorized to work in the united states?` 36,058
· `do you currently reside in the united states?` 9,420 · `are you authorized to work in
the united states?` 9,157 · `will you now or in the future require sponsorship for
employment visa status?` 8,413 · `will you now or in the future require visa sponsorship?`
7,587. Family total **70,635**.

**How did you hear about us** — 28,222 + 22,148 + 11,392 = **61,762**.

**Age gate** — `are you at least 18 years of age?` 21,367 · `are you 18 years of age or
older?` 8,970. Family total **30,337**.

**Money and timing** — `what are your salary expectations?` 15,972 · `what is your desired
salary?` 8,790 (salary total **24,762**) · `what is your notice period?` 7,841.

## What these numbers do not support

- **The cut-off hides the tail.** `HAVING count(*) >= 3000` means the families above are
  the common questions, not all of them. A question asked by 200 employers is absent here
  and the article must not imply the list is exhaustive.
- **A label is not a required field.** These counts include optional questions; how much of
  a form is mandatory is `02-required-fields.md`'s question.
- **One label can be one employer repeated.** A company with 4,000 open postings puts its
  custom question on 4,000 forms. These are per-posting counts, not per-employer ones, and
  that is a real limit on reading them as "how many employers ask this."
