package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/atsboard"
	"github.com/strelov1/freehire/internal/ingest/atsdetect"
	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/job/qualityaudit"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/safehttp"
	"github.com/strelov1/freehire/internal/platform/scraplingfetch"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// The successor pass (-successors) runs on its own timer on the crawl host, where the Scrapling
// tier is, not in the weekly measuring run on core. It writes the moves for the SuperPlane "Board
// successors" canvas to check. With -apply-moved it is the one part of this audit that acts: a
// renamed tenant (renamedTenant) has its old board retired here, since nothing else ever would —
// its crawl only sees a 404 or "board gone", never where the board went.

// successorFileEnv names the JSON report's path; the canvas reads it.
const successorFileEnv = "QUALITY_AUDIT_SUCCESSORS_FILE"

// successorReport is the file's shape, a contract with the canvas prompt in freehire-ops.
type successorReport struct {
	GeneratedAt time.Time       `json:"generated_at"`
	WindowDays  int32           `json:"window_days"`
	Chronic     int64           `json:"chronic_boards"`
	Probed      int             `json:"probed_boards"`
	Moves       []successorMove `json:"moves"`
}

type successorMove struct {
	Provider     string    `json:"provider"`
	Board        string    `json:"board"`
	Region       string    `json:"region"`
	Company      string    `json:"company"`
	OpenJobs     int64     `json:"open_jobs"`
	FailingSince time.Time `json:"failing_since"`
	LastError    string    `json:"last_error"`
	ProbedURL    string    `json:"probed_url"`
	Target       string    `json:"target"`
	Evidence     string    `json:"evidence"`
	// SuccessorProvider/Board are empty when the target is not an ATS we recognise.
	SuccessorProvider string `json:"successor_provider"`
	SuccessorBoard    string `json:"successor_board"`
	// SuccessorStatus is the successor's catalogue status: "absent", "pending", "active" or
	// "retired"; "" when there is no recognised successor.
	SuccessorStatus  string `json:"successor_status"`
	SuccessorCompany string `json:"successor_company"`
	// SuccessorChronic is a successor that is itself among the unreachable boards.
	SuccessorChronic bool `json:"successor_chronic"`
	// Retired is set when -apply-moved retired the old board this run; ClosedJobs is what that closed.
	Retired    bool  `json:"retired"`
	ClosedJobs int64 `json:"closed_jobs"`
}

// renamedTenant is a move the pass applies itself: the platform says the old board is gone
// (goneEvidence) and its pages point at the same provider's board, live in the catalogue and
// crawling. Retiring the old board stops a crawl that can only fail; its postings are already
// carried by the successor. A board failing for our own reasons (a WAF, a page cap) is never
// retired here, even when a posting page redirects: on CareerPlug a franchise's postings redirect
// to the network's board while the franchise's own board stays live (2026-11-26). Every other move
// needs a human: another platform, or a board the catalogue lacks.
func renamedTenant(m successorMove) bool {
	return m.SuccessorProvider == m.Provider && m.SuccessorStatus == "active" && !m.SuccessorChronic &&
		goneEvidence.MatchString(m.LastError)
}

// goneEvidence is the platform itself saying the board is gone: the adapters' ErrBoardGone text,
// a 404/410 on the board's listing, or its host no longer resolving.
// shortcut: matches board_health.last_error text; a structured gone reason on board_health would
// be sturdier if an adapter's wording changes.
var goneEvidence = regexp.MustCompile(`board no longer exists|status 404|status 410|no such host`)

// moveApplier retires one move's old board if, at the moment of the write, the move still holds.
type moveApplier interface {
	RetireIfStillMoved(ctx context.Context, m successorMove) (r boardcatalog.Retirement, held bool, err error)
}

// txApplier re-checks a move and retires it in one transaction. The old board's board_health row
// is locked first, so a crawl recording a success meanwhile waits for the commit, and the check
// reads what is true now rather than what the network pass saw minutes earlier.
type txApplier struct {
	pool   *pgxpool.Pool
	window time.Duration
}

func (a txApplier) RetireIfStillMoved(ctx context.Context, m successorMove) (boardcatalog.Retirement, bool, error) {
	var out boardcatalog.Retirement
	held := false
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		health, err := q.LockSuccessorAuditBoardHealth(ctx, db.LockSuccessorAuditBoardHealthParams{
			Provider: m.Provider, Board: m.Board, Region: m.Region,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // already retired, or its health row is gone: nothing to retire
		}
		if err != nil {
			return err
		}
		if !health.LastSeen.Valid || time.Since(health.LastSeen.Time) < a.window || !goneEvidence.MatchString(health.LastError) {
			return nil // the board recovered, or its latest failure is not the platform's "gone"
		}
		next, err := catalogBoard(ctx, q, m.SuccessorProvider, m.SuccessorBoard)
		if err != nil {
			return err
		}
		if next.Status != "active" {
			return nil
		}
		held = true
		out, err = boardcatalog.NewQueriesRepository(q).Retire(ctx, m.Provider, m.Board, m.Region)
		return err
	})
	return out, held, err
}

// applyMoves retires the old board of every renamed tenant in the report, at most maxRetire of
// them: more in one run than a renamed-tenant day ever produced is worth a human looking first.
// A move that no longer holds at the write is left alone. On an error the moves applied so far stay
// marked in the report, so the caller can still publish them.
func applyMoves(ctx context.Context, applier moveApplier, report *successorReport, maxRetire int, apply bool) error {
	n := 0
	for i := range report.Moves {
		m := &report.Moves[i]
		if !renamedTenant(*m) {
			continue
		}
		if n >= maxRetire {
			log.Printf("quality-audit: successors: retire cap %d reached; %s/%s left for the next run", maxRetire, m.Provider, m.Board)
			continue
		}
		n++
		if !apply {
			log.Printf("quality-audit: successors: would retire %s/%s, moved to %s/%s (-apply-moved to retire)", m.Provider, m.Board, m.SuccessorProvider, m.SuccessorBoard)
			continue
		}
		r, held, err := applier.RetireIfStillMoved(ctx, *m)
		if err != nil {
			return fmt.Errorf("retire %s/%s: %w", m.Provider, m.Board, err)
		}
		if !held {
			log.Printf("quality-audit: successors: %s/%s no longer qualifies at the write; left alone", m.Provider, m.Board)
			continue
		}
		m.Retired, m.ClosedJobs = r.Retired, r.Closed
		log.Printf("quality-audit: successors: retired %s/%s (moved to %s/%s), closed %d posting(s)", m.Provider, m.Board, m.SuccessorProvider, m.SuccessorBoard, r.Closed)
	}
	return nil
}

func runSuccessors(dryRun, applyMoved bool) int {
	path := os.Getenv(successorFileEnv)
	if path == "" && !dryRun {
		log.Printf("quality-audit: %s is unset, nothing to write", successorFileEnv)
		return worker.Skip(successorFileEnv)
	}
	days, err := worker.EnvInt32("QUALITY_AUDIT_SUCCESSOR_DAYS", 7)
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	maxBoards, err := worker.EnvInt32("QUALITY_AUDIT_SUCCESSOR_MAX", 2000)
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	maxRequests, err := worker.EnvInt64("QUALITY_AUDIT_SUCCESSOR_MAX_REQUESTS", 8000)
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	finder := &qualityaudit.SuccessorFinder{
		Direct:        safehttp.NewClient(probeTimeout),
		Name:          nameBoard,
		MaxRequests:   maxRequests,
		HostInterval:  hostInterval,
		MaxCrawlDelay: maxCrawlDelay,
		HostWorkers:   hostWorkers,
	}
	if raw := strings.TrimSpace(os.Getenv("SCRAPLING_FETCH_URL")); raw != "" {
		tr, err := scraplingfetch.New(scraplingfetch.Config{
			URL: raw, Token: strings.TrimSpace(os.Getenv("SCRAPLING_FETCH_TOKEN")),
		}, scraplingfetch.ModeHTTP, false, time.Minute)
		if err != nil {
			log.Printf("config: %v", err)
			return 1
		}
		finder.Fallback = &http.Client{Transport: tr, Timeout: time.Minute}
	} else {
		log.Print("quality-audit: successors: SCRAPLING_FETCH_URL unset, refused hosts are not retried")
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	q := db.New(pool)

	report, err := findSuccessors(ctx, q, finder, days, maxBoards)
	if err != nil {
		log.Printf("quality-audit: successors: %v", err)
		return 1
	}
	maxRetire, err := worker.EnvInt32("QUALITY_AUDIT_SUCCESSOR_MAX_RETIRE", 50)
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}
	applier := txApplier{pool: pool, window: time.Duration(days) * 24 * time.Hour}
	applyErr := applyMoves(ctx, applier, &report, int(maxRetire), applyMoved && !dryRun)
	if applyErr != nil {
		// The report is still written below: it records the retirements that did happen.
		log.Printf("quality-audit: successors: %v", applyErr)
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Printf("quality-audit: successors: %v", err)
		return 1
	}
	if dryRun {
		fmt.Println(string(body))
		return 0 // a dry run applies nothing, so applyErr is always nil here
	}
	if err := worker.WriteTextfile(filepath.Dir(path), filepath.Base(path), string(body)+"\n"); err != nil {
		log.Printf("quality-audit: successors: %v", err)
		return 1
	}
	if applyErr != nil {
		return 1
	}
	return 0
}

// nameBoard is the catalogue board a page is: its URL through atsboard, else a self-hosted
// career site's platform marker (body empty skips the marker).
func nameBoard(u *url.URL, body string) (provider, board string, ok bool) {
	if src, b, _, ok := atsboard.Recognize(u.String()); ok {
		return src, b, true
	}
	if body == "" {
		return "", "", false
	}
	return atsdetect.DetectSelfHosted(body, u.Hostname())
}

type successorQueries interface {
	ListChronicBoards(context.Context, db.ListChronicBoardsParams) ([]db.ListChronicBoardsRow, error)
	SuccessorAuditBoardJobs(context.Context, db.SuccessorAuditBoardJobsParams) (db.SuccessorAuditBoardJobsRow, error)
	GetBoardForResolve(context.Context, db.GetBoardForResolveParams) (db.Board, error)
}

type successorProber interface {
	Run(context.Context, []qualityaudit.ChronicBoard) []qualityaudit.Move
}

// findSuccessors probes a random maxBoards of the boards unreachable for days, so a capped run
// reaches a different share of a large backlog each time, and resolves each move's successor
// against the catalogue.
func findSuccessors(ctx context.Context, q successorQueries, finder successorProber, days, maxBoards int32) (successorReport, error) {
	report := successorReport{GeneratedAt: time.Now().UTC(), WindowDays: days, Moves: []successorMove{}}
	rows, err := q.ListChronicBoards(ctx, db.ListChronicBoardsParams{
		AgeWindow: pgtype.Interval{Days: days, Valid: true}, MaxBoards: 100000,
	})
	if err != nil {
		return report, fmt.Errorf("list chronic boards: %w", err)
	}
	if len(rows) > 0 {
		report.Chronic = rows[0].Total
	}
	chronic := make(map[string]bool, len(rows))
	for _, r := range rows {
		chronic[r.Provider+"/"+strings.ToLower(r.Board)] = true
	}
	rand.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] }) //nolint:gosec // Only selects which boards a capped run probes.

	var boards []qualityaudit.ChronicBoard
	for _, r := range rows {
		if len(boards) >= int(maxBoards) {
			break
		}
		if r.Board == "" { // a boardless provider's own record has no board to move
			continue
		}
		jobs, err := q.SuccessorAuditBoardJobs(ctx, db.SuccessorAuditBoardJobsParams{
			Source: r.Provider, BoardPattern: externalid.BoardPattern(r.Board),
		})
		if err != nil {
			return report, fmt.Errorf("jobs of %s/%s: %w", r.Provider, r.Board, err)
		}
		if jobs.LatestUrl == "" {
			continue
		}
		old, err := catalogBoard(ctx, q, r.Provider, r.Board)
		if err != nil {
			return report, err
		}
		since := r.FirstSeenAt
		if r.LastSuccessAt.Valid {
			since = r.LastSuccessAt
		}
		boards = append(boards, qualityaudit.ChronicBoard{
			Provider: r.Provider, Board: r.Board, Region: r.Region, Company: old.Company,
			OpenJobs: jobs.OpenJobs, LatestURL: jobs.LatestUrl, FailingSince: since.Time, LastError: r.LastError.String,
		})
	}
	report.Probed = len(boards)
	log.Printf("quality-audit: successors: probing %d of %d chronic boards", len(boards), report.Chronic)

	for _, m := range finder.Run(ctx, boards) {
		out := successorMove{
			Provider: m.Board.Provider, Board: m.Board.Board, Region: m.Board.Region, Company: m.Board.Company,
			OpenJobs: m.Board.OpenJobs, FailingSince: m.Board.FailingSince, LastError: m.Board.LastError,
			ProbedURL: m.ProbedURL, Target: m.Target, Evidence: m.Evidence,
			SuccessorProvider: m.SuccessorProvider, SuccessorBoard: m.SuccessorBoard,
		}
		if m.SuccessorBoard != "" {
			next, err := catalogBoard(ctx, q, m.SuccessorProvider, m.SuccessorBoard)
			if err != nil {
				return report, err
			}
			out.SuccessorStatus, out.SuccessorCompany = next.Status, next.Company
			out.SuccessorChronic = chronic[m.SuccessorProvider+"/"+strings.ToLower(m.SuccessorBoard)]
		}
		log.Printf("quality-audit: successors: %s/%s -> %s (%s/%s, %s)", out.Provider, out.Board, out.Target,
			out.SuccessorProvider, out.SuccessorBoard, out.SuccessorStatus)
		report.Moves = append(report.Moves, out)
	}
	log.Printf("quality-audit: successors: %d move(s) found", len(report.Moves))
	return report, ctx.Err()
}

// catalogBoard reads a board's catalogue row, reporting status "absent" when there is none.
func catalogBoard(ctx context.Context, q successorQueries, provider, board string) (db.Board, error) {
	b, err := q.GetBoardForResolve(ctx, db.GetBoardForResolveParams{Provider: provider, Board: board})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Board{Status: "absent"}, nil
	}
	if err != nil {
		return db.Board{}, fmt.Errorf("catalogue %s/%s: %w", provider, board, err)
	}
	return b, nil
}
