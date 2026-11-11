package rederive

import (
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestRowRepairsRemoteContradictedByStoredHybridArrangement(t *testing.T) {
	params, changed, _ := Row(db.Job{
		ID: 42, Source: "viterbit", ExternalID: "ad4-octogono:qualys",
		Title: "Especialista Senior en Ciberseguridad", Company: "AD4 Octogono",
		Location: "Barcelona, ES", Description: "Ubicación: Barcelona (Modelo Híbrido)",
		WorkMode: "remote",
	}, nil)
	if params.WorkMode != "hybrid" || !changed {
		t.Errorf("WorkMode = %q, changed = %v; want hybrid and changed", params.WorkMode, changed)
	}
}
