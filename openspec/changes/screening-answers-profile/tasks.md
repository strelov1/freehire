## 1. Storage

- [ ] 1.1 Add migration `0092_screening_answers.sql`: `screening_answers` table, one row per
      user (`user_id bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE`), the six
      nullable columns (`authorized_countries text[]`, `visa_sponsorship_needed boolean`,
      `desired_salary_amount integer`, `desired_salary_currency text`,
      `desired_salary_period text`, `notice_period_days integer`,
      `willing_to_relocate boolean`, `age_18_or_older boolean`), `updated_at timestamptz`.
- [ ] 1.2 Add `internal/db/queries/screening_answers.sql` (get-by-user, upsert-partial) and
      run `make sqlc`.
- [ ] 1.3 Create `internal/screeninganswers` package: wire shape, `Sanitize`/`Validate`
      (country codes against `internal/location`, currency against the `salary_currency`
      vocab), owner-scoped `Store` over the generated queries.
- [ ] 1.4 Add `internal/screeninganswers/AGENTS.md` documenting the domain boundary
      decisions from design.md (why not `userprofile`/`experience`/`resumeextract`, no
      provenance state machine).

## 2. Manual read/write endpoint

- [ ] 2.1 Add `GET /me/screening-answers` and `PUT /me/screening-answers` handlers
      (`internal/handler/screening_answers.go`), partial-update semantics, `{"data": ...}` /
      `{"error": ...}` response shapes per repo convention.
- [ ] 2.2 Register routes and wire the store into `internal/handler/handler.go`.
- [ ] 2.3 Run `cmd/gen-contracts` to emit the TypeScript wire types for the web client.

## 3. Assistant tool

- [ ] 3.1 Add `internal/handler/assistant_screening_tools.go`: `screening_answers_set` tool
      accepting a partial set of the six fields, calling the same store as the manual-edit
      handler, returning the fields it wrote.
- [ ] 3.2 Register the tool in `internal/handler/assistant_tools.go` under the presets that
      should offer it (candidate-facing chat presets).
- [ ] 3.3 Error messages name the invalid value and the valid set, per the "Adding a tool"
      convention in `internal/assistant/AGENTS.md`.

## 4. Autofill integration

- [ ] 4.1 Extend `autofillProfile` (`internal/handler/autofill_profile.go`) to read the
      caller's screening answers and format them as human-readable strings (e.g.
      `"1 month"`, `"120,000 USD/year"`, `"yes"`), added to the struct and to the
      `Profile map[string]string` the agent plans against.
- [ ] 4.2 Update `internal/autofillagent/planner.go`: remove the `choosePrompt` line stating
      visa/notice-period/salary/relocation questions are never answered by a profile, since
      it is no longer true.
- [ ] 4.3 Extend `internal/autofillagent` tests/fixtures to cover a screening-question field
      being planned and chosen from the profile.

## 5. Web: manual edit surface

- [ ] 5.1 Add a "Screening answers" section to `web/src/routes/my/profile`, visually and
      structurally separate from the existing skills/specializations form (own heading, own
      save action).
- [ ] 5.2 Wire the section to `GET`/`PUT /me/screening-answers` using the generated
      contract types.

## 6. Verification

- [ ] 6.1 Unit tests for `internal/screeninganswers` (`Sanitize`/`Validate`, partial-update
      semantics, dict rejection for country/currency).
- [ ] 6.2 Integration test for the manual-edit endpoints (`-tags=integration`).
- [ ] 6.3 Manual verification: set screening answers, run agent-driven autofill against a
      fixture form with a matching screening question, confirm the plan includes it.
