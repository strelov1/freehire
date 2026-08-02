package userjob

// StageCount is one (stage, count) group from the per-user application aggregate.
// An empty Stage is an applied row carrying no explicit stage.
type StageCount struct {
	Stage string
	Count int64
}

// Pipeline is the application-pipeline snapshot: the total application count and the count at
// each stage of the vocabulary. Stage counts always sum to Applications.
//
// It reports stages rather than the seven coarse buckets it used to, because those bucket names
// (no_answer, in_progress, declined) were a third vocabulary for one state — a name the reader
// met nowhere else in the product. Grouping stages into the four the board shows is the job of
// Groups, applied by whoever renders, so the mapping exists once instead of once per surface.
type Pipeline struct {
	Applications int64            `json:"applications"`
	Stages       map[string]int64 `json:"stages"`
}

// CountByStage folds per-stage rows into the pipeline snapshot.
//
// Every stage of the vocabulary is present, zero included: a key that appears only when non-zero
// makes a caller distinguish "nobody is here" from "the server forgot this one", and the SPA
// would have to carry its own list of stages to tell them apart — the copy this change removes.
//
// A row whose stage is empty or outside the vocabulary is counted under `applied`. Empty is the
// ordinary case (applied, no explicit stage) and belongs there. Out-of-vocabulary is an anomaly
// — `stage` is free text with no CHECK — and folding it keeps the sum honest, where dropping it
// would make the parts disagree with the total for a reason no reader could see.
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
