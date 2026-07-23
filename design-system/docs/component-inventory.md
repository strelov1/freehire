# Component inventory

Living doc. Inventories `.svelte` files under `web/src/lib/components/` (root + `cv/`, `facets/`, `filters/`, `onboarding/`) and `web/src/lib/ui/`.

Files are classified into four buckets — **components**, **views**, **layouts**, **patterns**. Only **components** are the focus of this inventory; the other three are listed for reference and deferred to Storybook (phase 05), where their stories can capture the recurring shapes without conflating them with reusable primitives.

Last updated: post-phase-1 refinement (component vs. view distinction).

## Components (29)

Self-contained UI building blocks with a defined props/slots API. Not bound to a specific route or page. These are the candidates for the design-system primitive layer (phase 04) and for Storybook stories (phase 05).

### Existing primitives (`web/src/lib/ui/`)

Migrate into `design-system/src/` in phase 04. All use Svelte 5 runes + `tailwind-variants`.

| file | notes |
|---|---|
| button.svelte | variant: primary/secondary/outline/ghost; size: sm/md/lg/icon. `tv` with `buttonVariants`. |
| badge.svelte | variant: secondary/outline/brand. `tv` with `badgeVariants`. |
| input.svelte | `$bindable` value, `class` pass-through via `cn()`. Focus ring via `ring-ring/50`. |
| skeleton.svelte | Pure `animate-pulse rounded-md bg-muted` div. |

### Generic primitives (not bound to a domain)

| file | subdir | classification | reason |
|---|---|---|---|
| Avatar.svelte | | primitive candidate | Generic email-initial avatar circle with deterministic color. Formalize in phase 04. |
| CompanyLogo.svelte | | primitive candidate | Logo-with-monogram-fallback. Reusable across job row, job page, company page, header search. |
| ProviderIcon.svelte | | bespoke | Brand SVG icon set (Google/GitHub/Telegram/LinkedIn). Bespoke — Lucide has no brand marks. |
| BrandMark.svelte | | bespoke | freehire brand mark SVG. Brand asset, not a generic primitive. |
| States.svelte | | primitive candidate | Shared loading/empty/error rendering. Formalize as "Empty state" in phase 04. |
| LoadMore.svelte | | primitive candidate | Generic "Load more" button + error line. |
| InfiniteScroll.svelte | | primitive candidate | Pure IntersectionObserver sentinel trigger. |
| FilterEdgeTab.svelte | | primitive candidate | Generic floating edge-tab button. |
| DocsCodeBlock.svelte | | primitive candidate | Generic copyable code block (label + pre/Shiki + copy button). |
| StringListEditor.svelte | cv | primitive candidate | Generic add/remove list-of-strings editor over Input + Button. |
| NoteEditor.svelte | | domain component | EasyMDE wrapper. Generic markdown editor, but only used in JobDrawer today. |

### Data-viz components (presentational, need prop generalization)

All hand-built SVG. Presentational but bound to specific data shapes — generalizing their props would make them design-system chart primitives.

| file | classification | reason |
|---|---|---|
| RateDonut.svelte | primitive candidate | Donut chart with generic percent/label props. Already the cleanest. |
| ActivityBars.svelte | needs generalization | Grouped bar chart bound to `ActivityPoint` series. |
| GrowthArea.svelte | needs generalization | Area chart bound to `UserGrowthPoint` series. |
| PipelineFunnel.svelte | needs generalization | Sankey chart bound to `PIPELINE_BUCKETS`. |
| HomeFunnel.svelte | needs generalization | Near-duplicate of PipelineFunnel. Consolidate into shared `SankeyFunnel` taking `buckets: {key,label,color}[]`. |
| FacetBreakdown.svelte | needs generalization | Bar chart bound to `FacetDef` distribution. |

### Domain components (self-contained but bound to facet/filter system)

| file | subdir | classification | reason |
|---|---|---|---|
| SearchSelect.svelte | facets | pattern candidate | Searchable multi-select of three-state pills. Generic shape, but imports `FacetOption`. |
| RemoteSearchSelect.svelte | facets | pattern candidate | Server-backed searchable multi-select. Generic shape, but imports `FacetOption`. |
| TokenInput.svelte | facets | primitive candidate | Free-text chip input (Enter add / Backspace remove). Genuinely generic. |
| PillGroup.svelte | facets | pattern candidate | Three-state pill group. Imports `FacetOption`. The pill shape is a primitive candidate; the three-state facet logic is a pattern. |
| FacetHeader.svelte | filters | pattern candidate | Label + Clear header. Imports `FacetStore`. The header shape is reusable; the store coupling is a pattern. |
| SaveSearchAlert.svelte | filters | domain component | Save-search + Telegram-alert control. Reused across 4 pages but bound to saved-search store. |
| AuthDialog.svelte | | pattern candidate | Modal hand-rolling backdrop + Escape + scroll-lock. The Dialog *chrome* is a primitive candidate; the auth form is a view. |
| ReportDialog.svelte | | pattern candidate | Multi-step modal. Same Dialog chrome candidate; the report form is a view. |
| GmailConnectDialog.svelte | | pattern candidate | One-off modal. Same Dialog chrome candidate; content is a view. |
| ApplicationLinkPicker.svelte | | pattern candidate | Searchable popover for InboxView. Popover/Select primitive candidate; the link-picker logic is a view. |
| GithubStars.svelte | | domain component | Star-count badge bound to `githubStars` store. |
| RealityBadge.svelte | | domain component | Badge bound to `Reality` signal. |

## Views (35) — deferred to Storybook (phase 05)

Page-level compositions that wire data to APIs. Not reusable UI building blocks.

`ATSReportView`, `AboutValues`, `AnalysesView`, `AnalyticsView`, `ApiKeysView`, `BoardCard`, `BoardColumn`, `ChatGptView`, `CliView`, `CompaniesView`, `CompanyAbout`, `CompanyFacts`, `CompanyFollowButton`, `CompanyHeader`, `CompanyView`, `ContributeLandingView`, `ContributeView`, `DocsEndpoint`, `ForCompaniesView`, `HomeView`, `InboxView`, `JobBoard`, `JobDrawer`, `JobFitAnalysis`, `JobFitFull`, `JobHistory`, `JobMatch`, `JobRelated`, `JobRow`, `JobView`, `JobsView`, `ModerationView`, `MySubmissionsView`, `PipelineView`, `ProfileForm`, `ResumeStructuredView`, `SavedSearchesView`, `StatusBoard`, `SubmitView`, `SwipeDeck`, `VerdictView`, `CvEditor`, `CvList`

## Layouts (10) — deferred to Storybook (phase 05)

Structural shells: headers, footers, page chrome, navigation, head metadata.

`TopBar`, `Footer`, `HeaderMenu`, `HeaderSearch`, `HeaderListSearch`, `HeaderLocationFilter`, `ListToolbar`, `InsightsPageShell`, `DocsNav`, `Seo`

## Patterns (20) — deferred to Storybook (phase 05)

Recurring multi-component compositions. Not single components — they combine multiple primitives into an interaction (filter modal = Dialog + FacetSection + PillGroup + footer; list+paginator = States + JobRow + LoadMore).

**Filter system:** `FilterModalShell`, `FilterSummaryShell`, `FilterModal`, `FilterSummary`, `CompanyFilterModal`, `CompanyFilterSummary`
**Facet controls:** `FacetSection`, `ChipFacet`, `CategoryPane`, `LocationPane`
**Tracking lists:** `SavedJobs`, `SavedSearches`, `ReportQueue`
**Onboarding flow:** `OnboardingWizard`, `OnboardingBanner`, `OnboardingAlertBanner`

## Migration-candidate patterns (for phase 04 reference)

These are the recurring shapes the design-system primitive layer should absorb. Each is a pattern that multiple views/layouts hand-roll today.

1. **Dialog/modal chrome — 3 implementations.** `AuthDialog`, `ReportDialog`, `GmailConnectDialog` each hand-roll backdrop + Escape + scroll-lock. Extract into a shared `<Dialog>` primitive.
2. **Data-viz — 6 implementations.** `ActivityBars`, `GrowthArea`, `HomeFunnel`, `PipelineFunnel`, `RateDonut`, `FacetBreakdown`. `RateDonut` is the only generic one; `PipelineFunnel` + `HomeFunnel` are near-duplicates.
3. **Searchable select — 4 variants.** `SearchSelect`, `RemoteSearchSelect`, `TokenInput`, `ApplicationLinkPicker`. Overlapping "type → filter → pick" pattern.
4. **Tabs — 5 inline implementations** (in views: `JobRelated`, `JobDrawer`, `ModerationView`, `VerdictView`, `ProfileForm`). Shared `<Tabs>` primitive absorbs all.
5. **List + paginator — 3 near-identical** (in views: `SavedJobs`, `JobHistory`, `JobsView`/`CompaniesView`). `<PaginatedList>` wrapper.
6. **Marketing "steps" — 5 copies** (in views: `AboutValues`, `HomeView`, `ContributeLandingView`, `ForCompaniesView`, `RecruitersView`). `<NumberedSteps>` block.
