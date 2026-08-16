# Sources

The [README](../README.md) covers what freehire is; this is the full per-source
breakdown behind the "3.3M+ postings" headline. Live catalogue snapshot —
**3,300,615 open postings** across **294,282 companies** from
**225** sources. Counts are open postings unless noted; a company crawled
from two sources is counted under each, so the per-section company totals below
sum to more than the distinct figure above. Every source is one of three kinds:

- **ATS platforms** — one adapter per multi-tenant applicant-tracking system,
  each serving many companies (Workday, Greenhouse, Lever, iCIMS…).
- **Aggregators & job boards** — third-party feeds that republish many
  companies' postings (Adzuna, trudvsem, himalayas, jobtech, Telegram…).
- **Company career sites** — direct single-company feeds crawled from a
  company's own careers page (Amazon, Apple, Google, Yandex, Sber…).

A source's kind is not configuration: it falls out of the adapter's own Go type
in `internal/sources`, so the split below is derived, not maintained by hand.

## ATS platforms

**92 platforms · 157,399 companies · 2,644,796 open postings.**

| Source | Companies | Open jobs |
| --- | ---: | ---: |
| workday | 4,202 | 748,235 |
| trudvsem | 70,228 | 266,965 |
| smartrecruiters | 2,810 | 218,150 |
| oracle | 636 | 205,930 |
| greenhouse | 6,863 | 155,528 |
| ukg | 1,897 | 151,597 |
| icims | 3,062 | 77,788 |
| paycom | 5,804 | 69,672 |
| ashby | 3,994 | 56,647 |
| phenom | 92 | 53,137 |
| gupy | 1,429 | 52,408 |
| bamboohr | 9,120 | 50,674 |
| lever | 2,205 | 50,293 |
| workable | 1,480 | 40,563 |
| recruitee | 2,037 | 39,521 |
| teamtailor | 1,899 | 36,452 |
| personio | 4,108 | 35,342 |
| jazzhr | 3,940 | 34,285 |
| paylocity | 2,635 | 23,303 |
| breezy | 1,753 | 20,902 |
| zohorecruit | 1,203 | 20,185 |
| eightfold | 45 | 19,706 |
| apploi | 2,957 | 17,742 |
| applicantpro | 1,802 | 16,186 |
| hireology | 2,293 | 15,364 |
| pinpoint | 806 | 12,954 |
| solides | 1,132 | 12,406 |
| careerplug | 4,515 | 12,324 |
| radancy | 575 | 12,313 |
| isolvedhire | 2,043 | 12,149 |
| jibe | 46 | 11,984 |
| join | 4,260 | 9,393 |
| manatal | 141 | 9,230 |
| taleo | 27 | 7,818 |
| inhire | 368 | 7,034 |
| trakstar | 665 | 6,549 |
| freshteam | 214 | 6,065 |
| jobvite | 149 | 4,667 |
| arbeitsagentur | 2,080 | 4,499 |
| successfactors | 12 | 4,401 |
| factorial | 482 | 4,270 |
| rippling | 320 | 3,256 |
| gem | 250 | 2,461 |
| erecruiter | 30 | 2,434 |
| traffit | 49 | 2,161 |
| neogov | 16 | 1,842 |
| senior | 83 | 1,591 |
| likeit | 16 | 1,466 |
| peopleforce | 77 | 1,458 |
| cornerstone | 15 | 1,421 |
| avature | 4 | 1,328 |
| catsone | 21 | 1,086 |
| betterteam | 150 | 1,007 |
| pageup | 8 | 978 |
| epam | 1 | 954 |
| hibob | 67 | 911 |
| softgarden | 20 | 867 |
| luxoft | 1 | 747 |
| loxo | 12 | 672 |
| comeet | 23 | 625 |
| deel | 65 | 580 |
| wpyoast | 1 | 372 |
| clinch | 1 | 363 |
| compleo | 4 | 179 |
| huntflow | 24 | 176 |
| opencats | 9 | 164 |
| workablemarketplace | 2 | 108 |
| ismartrecruit | 2 | 102 |
| ashbygraphql | 3 | 101 |
| jobscore | 6 | 92 |
| cleverstaff | 40 | 86 |
| talentlyft | 8 | 73 |
| bullhorn | 2 | 69 |
| hurma | 7 | 51 |
| earcu | 1 | 50 |
| globalpayments | 1 | 45 |
| quickin | 5 | 41 |
| careerspage | 3 | 41 |
| recruitingsolutions | 17 | 40 |
| vention | 1 | 34 |
| rapyd | 1 | 28 |
| northstone | 3 | 21 |
| adp | 1 | 17 |
| speedrun | 10 | 16 |
| odoo | 1 | 12 |
| vouch | 1 | 10 |
| mindsight | 1 | 8 |
| spark | 1 | 7 |
| enlizt | 2 | 6 |
| talenthr | 2 | 4 |
| talentadore | 1 | 2 |
| briefhq | 1 | 2 |

## Aggregators & job boards

**100 sources · 172,884 companies · 629,571 open postings.**

| Source | Companies | Open jobs |
| --- | ---: | ---: |
| adzuna | 25,171 | 159,372 |
| mycareersfuture | 23,094 | 70,203 |
| echojobs | 5,922 | 49,590 |
| jobtech | 11,344 | 31,103 |
| infojobs | 16,667 | 22,690 |
| jobnet | 8,912 | 16,326 |
| gulftalent | 765 | 15,308 |
| whatjobs-uk | 3,948 | 14,192 |
| himalayas | 8,060 | 13,845 |
| tyomarkkinatori | 4,401 | 12,613 |
| 4dayweek | 672 | 10,518 |
| whatjobs | 3,846 | 10,090 |
| whatjobs-sg | 2,084 | 9,643 |
| whatjobs-es | 1,674 | 9,542 |
| hh | 6,195 | 9,375 |
| justjoin | 1,295 | 9,036 |
| reed | 1,664 | 8,921 |
| telegram | 2,985 | 8,177 |
| usajobs | 395 | 8,114 |
| whatjobs-ca | 2,405 | 7,742 |
| jobylon | 939 | 7,613 |
| jobdanmark | 2,888 | 7,404 |
| djinni | 2,417 | 6,719 |
| whatjobs-pl | 1,732 | 6,572 |
| jobstash | 925 | 6,571 |
| wantedkr | 2,693 | 5,902 |
| powertofly | 34 | 5,872 |
| whatjobs-ph | 1,721 | 5,400 |
| whatjobs-fr | 1,554 | 5,295 |
| whatjobs-za | 869 | 5,277 |
| whatjobs-it | 1,215 | 5,215 |
| arbeitnow | 2,909 | 4,978 |
| whatjobs-nl | 1,455 | 4,696 |
| workatastartup | 1,374 | 4,254 |
| nofluffjobs | 452 | 3,189 |
| whatjobs-au | 1,168 | 3,120 |
| whatjobs-mx | 837 | 2,510 |
| whatjobs-co | 586 | 2,499 |
| whatjobs-ae | 768 | 2,414 |
| whatjobs-id | 880 | 2,304 |
| whatjobs-my | 823 | 2,151 |
| whatjobs-pk | 651 | 2,118 |
| whatjobs-ie | 648 | 1,950 |
| vagas | 527 | 1,753 |
| whatjobs-ar | 414 | 1,547 |
| whatjobs-hk | 407 | 1,497 |
| whatjobs-vn | 698 | 1,433 |
| whatjobs-br | 632 | 1,403 |
| aijobs | 680 | 1,384 |
| remoteok | 1,150 | 1,326 |
| whatjobs-sa | 423 | 1,317 |
| whatjobs-ch | 501 | 1,273 |
| instaffo | 654 | 1,262 |
| getonbrd | 355 | 1,180 |
| whatjobs-de | 393 | 1,151 |
| thehub | 348 | 949 |
| whatjobs-cl | 291 | 923 |
| whatjobs-dk | 382 | 878 |
| getmatch | 156 | 855 |
| whatjobs-qa | 194 | 840 |
| functionalworks | 326 | 769 |
| whatjobs-fi | 259 | 676 |
| habr_career | 148 | 670 |
| whatjobs-nz | 231 | 532 |
| whatjobs-no | 259 | 528 |
| whatjobs-pe | 169 | 489 |
| whatjobs-tr | 230 | 482 |
| jobicy | 313 | 407 |
| whatjobs-in | 104 | 399 |
| getro | 119 | 363 |
| weworkremotely | 286 | 340 |
| wantapply | 92 | 295 |
| whatjobs-pt | 104 | 280 |
| whatjobs-be | 108 | 262 |
| geekjob | 142 | 227 |
| whatjobs-se | 123 | 210 |
| remotli | 25 | 158 |
| whatjobs-kw | 52 | 141 |
| whatjobs-om | 65 | 122 |
| whatjobs-bh | 48 | 98 |
| whatjobs-lu | 36 | 89 |
| whatjobs-hu | 45 | 86 |
| startupandvc | 71 | 80 |
| whatjobs-at | 43 | 61 |
| whatjobs-ve | 32 | 59 |
| tecla | 39 | 57 |
| workingnomads | 21 | 53 |
| whatjobs-th | 14 | 45 |
| getmanfred | 38 | 45 |
| remotive | 23 | 36 |
| landingjobs | 11 | 32 |
| cryptocurrencyjobs | 29 | 32 |
| jobspresso | 15 | 19 |
| nodesk | 12 | 12 |
| topco | 5 | 10 |
| teamex | 1 | 8 |
| whatjobs-gr | 1 | 2 |
| whatjobs-ke | 1 | 1 |
| whatjobs-py | 1 | 1 |
| whatjobs-sv | 1 | 1 |

## Company career sites

**30 feeds · 59 companies · 26,222 open postings.**

| Source | Companies | Open jobs |
| --- | ---: | ---: |
| amazon | 1 | 7,680 |
| apple | 1 | 4,268 |
| google | 7 | 3,452 |
| alfabank | 1 | 2,208 |
| sber | 11 | 1,649 |
| mts | 12 | 1,188 |
| emagine | 1 | 988 |
| yandex | 1 | 846 |
| meta | 1 | 790 |
| uber | 1 | 606 |
| bairesdev | 1 | 539 |
| tbank | 1 | 465 |
| micro1 | 1 | 351 |
| vk | 1 | 262 |
| onstrider | 1 | 241 |
| avito | 1 | 162 |
| dataart | 1 | 124 |
| lamoda | 1 | 100 |
| rwb | 1 | 92 |
| alignerr | 1 | 55 |
| domclick | 1 | 32 |
| yandexcrowd | 1 | 25 |
| dodo | 3 | 21 |
| ozon | 1 | 20 |
| aviasales | 1 | 20 |
| 2gis | 1 | 15 |
| lumenalta | 1 | 11 |
| mtslink | 1 | 6 |
| telegramcareers | 1 | 5 |
| kuper | 1 | 1 |

Plus **26** postings from manually imported links and bulk imports.
