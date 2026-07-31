// Package followup assembles the message a candidate sends when an application has gone quiet.
//
// It is deterministic and I/O-free (the shape internal/atscheck and internal/verdict use): the same
// application drafts the same text, no model is called, and nothing here can invent a claim about
// the candidate. The one non-generic sentence is supplied by the caller from the cached fit
// analysis, which derived it from the candidate's own CV; when there is none, the line is omitted
// rather than filled.
//
// The tone rules are the product and are pinned by tests: no apology, no "just checking in", and a
// concrete question, because a chase nobody can answer is a nudge that reads as noise.
package followup

import "strings"

// Input is everything the draft is built from. Strength and RecipientName are optional; every
// other field is expected to be present for an application that has actually been sent.
type Input struct {
	Role       string
	Company    string
	DaysSilent int
	// Stage is the application's stage, which decides whether the opening says "I applied" or
	// "we last spoke" — a candidate who has interviewed did more than apply.
	Stage string
	// Strength is one concrete reason the candidate fits, taken from the cached fit analysis.
	// Empty means the analysis is missing or named none: the line is then dropped.
	Strength string
	// RecipientName is the display name from the linked mail's sender, when there is one.
	RecipientName string
}

// Message is the assembled draft. Comparable, so a test can assert two runs are identical.
type Message struct {
	Subject string
	Body    string
}

// Draft assembles the follow-up.
func Draft(in Input) Message {
	paragraphs := []string{greeting(in.RecipientName), opening(in)}
	if s := strings.TrimSpace(in.Strength); s != "" {
		paragraphs = append(paragraphs, "For context: "+strings.TrimRight(s, ".")+".")
	}
	paragraphs = append(paragraphs, ask(in.Stage), "Thanks for your time.")

	return Message{
		Subject: "Following up: " + in.Role + " application",
		// Joining a slice rather than appending to a buffer is what keeps an omitted strength from
		// leaving a blank paragraph behind.
		Body: strings.Join(paragraphs, "\n\n"),
	}
}

func greeting(name string) string {
	if n := strings.TrimSpace(name); n != "" {
		return "Hi " + n + ","
	}
	return "Hello,"
}

// opening states what happened and how long ago. A candidate past the application stage has spoken
// to somebody, and saying "I applied" there would understate the relationship.
func opening(in Input) string {
	where := "the " + in.Role + " role"
	if c := strings.TrimSpace(in.Company); c != "" {
		where += " at " + c
	}
	if in.Stage == "" || in.Stage == "applied" {
		return "I applied for " + where + " " + elapsed(in.DaysSilent) + " ago and have not had a reply yet."
	}
	return "We last spoke about " + where + " " + elapsed(in.DaysSilent) + " ago, and I have not heard since."
}

// ask matches the question to where the candidate actually is. Asking whether the role is still
// open after an interview reads as if they do not know they are already in the process.
func ask(stage string) string {
	if stage == "" || stage == "applied" {
		return "Is the role still open, and what is the timeline for the next step?"
	}
	return "Where does my application stand, and what are the next steps?"
}

// elapsed renders the silence in the units a person would use out loud: weeks up to about six, then
// months. The exact day count lives on the board; a message that says "24 days" reads like a system
// wrote it.
func elapsed(days int) string {
	if days >= 45 {
		return plural((days+15)/30, "month")
	}
	return plural((days+3)/7, "week")
}

func plural(n int, unit string) string {
	if n <= 1 {
		return "a " + unit
	}
	return numberWord(n) + " " + unit + "s"
}

func numberWord(n int) string {
	words := []string{"", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine"}
	if n < len(words) {
		return words[n]
	}
	return "several"
}
