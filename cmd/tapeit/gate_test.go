package main

import (
	"testing"

	"github.com/helmedeiros/tapeit/internal/curator"
)

func TestGateEvaluation(t *testing.T) {
	pass := curator.EvalResult{
		Playlists: 10, Recall: 0.14, BaselineRecall: 0.06,
		TrackPlaylists: 10, TrackRecall: 0.08, TrackBaselineRecall: 0.01,
	}
	if err := gateEvaluation(pass); err != nil {
		t.Errorf("healthy eval should pass, got %v", err)
	}

	artistRegression := pass
	artistRegression.Recall = 0.05 // now below baseline
	if err := gateEvaluation(artistRegression); err == nil {
		t.Error("artist recall below baseline should fail the gate")
	}

	trackRegression := pass
	trackRegression.TrackRecall = 0.005 // below track baseline
	if err := gateEvaluation(trackRegression); err == nil {
		t.Error("track recall below baseline should fail the gate")
	}
}
