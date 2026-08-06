## ADDED Requirements

### Requirement: Inline detail pane on the home feed (desktop)
On desktop viewport widths, the home feed SHALL present a compact job list alongside a detail pane in the same layout. Selecting a job from the list SHALL show that job's full detail in the pane without navigating away from the list.

#### Scenario: Selecting a job from the feed
- **WHEN** a user on the desktop home feed clicks a job card while a different (or no) job's detail is showing
- **THEN** the detail pane shows the selected job's full detail, the browser URL updates to that job's detail URL, and the list's scroll position and active filters are unchanged

#### Scenario: Selected card is visually marked
- **WHEN** a job's detail is showing in the pane
- **THEN** that job's card in the list is visually distinguished from the other, unselected cards

### Requirement: Job detail URLs remain unchanged and shareable
The per-job detail URL SHALL be unaffected by this layout change and SHALL continue to server-render that job's full detail when opened directly, with the list shown alongside on desktop.

#### Scenario: Opening a job link directly
- **WHEN** a user opens an existing job detail link with no prior visit to the site in this session
- **THEN** the page server-renders that job's full detail (and, at desktop width, the job list alongside it) without a client-side loading flash for the pane's primary content

### Requirement: Browser back/forward reflects selection history
Navigating browser back or forward across job selections made on the feed SHALL restore the detail pane and the address bar to the state that was current at that point in history, using the browser's native history mechanism rather than a client-side URL rewrite that can desync from it.

#### Scenario: Back after selecting two jobs in sequence
- **WHEN** a user selects job A, then selects job B, then presses the browser Back button
- **THEN** the detail pane shows job A's detail and the address bar matches job A's URL

### Requirement: Filters relocate above the list, behavior unchanged
The feed's filters SHALL be presented as a horizontal bar above the job list rather than a left-hand sidebar. Filtering behavior and URL-synced filter state SHALL be unchanged by the relocation.

#### Scenario: Applying a filter
- **WHEN** a user applies a filter from the horizontal filter bar
- **THEN** the list updates to match the filter, the URL reflects the filter, and an already-open detail pane, if any, is unaffected by the filter change

### Requirement: Compact list cards
Job cards in the feed's list column SHALL use the existing compact card presentation rather than the feed's current full-size cards.

#### Scenario: Card density in the split layout
- **WHEN** the home feed renders in the desktop two-column layout
- **THEN** each list card shows in the compact presentation, so more cards are visible without scrolling than the current full-size feed cards show

### Requirement: Mobile falls back to full-page detail
Below the desktop breakpoint, the home feed SHALL render the list alone, without a docked detail pane. Selecting a job SHALL navigate to its full-page detail route instead of opening an in-place pane.

#### Scenario: Selecting a job on a narrow viewport
- **WHEN** a user below the desktop breakpoint selects a job from the feed
- **THEN** the browser navigates to that job's full-page detail route, and no split layout is rendered

### Requirement: No forced re-analysis on selection
Selecting a job on the feed SHALL NOT trigger a new AI match analysis computation for a job that already has a cached analysis; the pane SHALL display the cached result, and computation SHALL only start from an explicit user action.

#### Scenario: Clicking through several already-analysed jobs
- **WHEN** a signed-in user with an existing CV clicks through five jobs on the feed, each of which already has a cached match analysis
- **THEN** no new AI analysis computation is started for any of the five jobs, and each pane shows its existing cached analysis
