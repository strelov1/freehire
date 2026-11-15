package userjob

// StageCount is one (stage, count) group from the per-user application aggregate.
// An empty Stage is an applied row carrying no explicit stage.
type StageCount struct {
	Stage string
	Count int64
}

// Pipeline is the application-pipeline snapshot: the total application count and the
// count at each stage of the vocabulary. Stage counts always sum to Applications.
// Grouping stages for display is Groups' job, so the mapping exists once.
type Pipeline struct {
	Applications int64            `json:"applications"`
	Stages       map[string]int64 `json:"stages"`
	// ReplyRate is the personal-vs-global employer reply-rate benchmark, present only
	// when GateReplyRateBenchmark clears both sides — never a zero or an estimate below
	// the sample gate, so omitempty is load-bearing here, not cosmetic.
	ReplyRate *ReplyRateBenchmark `json:"reply_rate,omitempty"`
}

// CountByStage folds per-stage rows into the pipeline snapshot.
//
// Every stage is present, zero included, so callers need not tell "nobody here" from
// "server forgot this one".
//
// An empty or out-of-vocabulary stage is counted under `applied`: empty is the
// ordinary case, and `stage` is free text with no CHECK, so folding keeps the parts
// summing to the total.
func CountByStage(counts []StageCount) Pipeline {
	stages := make(map[string]int64, len(Stages))
	for _, stage := range Stages {
		stages[stage] = 0
	}

	var p Pipeline
	for _, c := range counts {
		p.Applications += c.Count
		if !ValidStage(c.Stage) {
			stages["applied"] += c.Count
			continue
		}
		stages[c.Stage] += c.Count
	}
	p.Stages = stages
	return p
}
