package adapters

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/queue"
)

func TestScoreToProto_Nil(t *testing.T) {
	result := scoreToProto(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestScoreToProto_EmptyScore(t *testing.T) {
	s := &queue.Score{}
	result := scoreToProto(s)
	if result == nil {
		t.Fatal("expected non-nil for empty Score, got nil")
	}
	if result.TestResults != nil {
		t.Errorf("expected nil TestResults, got %v", result.TestResults)
	}
	if result.DiffSummary != nil {
		t.Errorf("expected nil DiffSummary, got %v", result.DiffSummary)
	}
	if result.RetryHistory != nil {
		t.Errorf("expected nil RetryHistory, got %v", result.RetryHistory)
	}
}

func TestScoreToProto_OnlyTestResults(t *testing.T) {
	s := &queue.Score{
		TestResults: &queue.TestResults{
			Passed:           true,
			OutputExcerpt:    "all tests passed",
			DurationMs:       1234,
			TestsRun:         10,
			TestsFailed:      0,
			FailingTestNames: nil,
		},
	}
	result := scoreToProto(s)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TestResults == nil {
		t.Fatal("expected non-nil TestResults")
	}
	if result.DiffSummary != nil {
		t.Errorf("expected nil DiffSummary, got %v", result.DiffSummary)
	}
	if result.RetryHistory != nil {
		t.Errorf("expected nil RetryHistory, got %v", result.RetryHistory)
	}
	if result.TestResults.Passed != true {
		t.Errorf("expected Passed=true, got false")
	}
	if result.TestResults.OutputExcerpt != "all tests passed" {
		t.Errorf("expected OutputExcerpt='all tests passed', got %q", result.TestResults.OutputExcerpt)
	}
	if result.TestResults.DurationMs != 1234 {
		t.Errorf("expected DurationMs=1234, got %d", result.TestResults.DurationMs)
	}
	if result.TestResults.TestsRun != 10 {
		t.Errorf("expected TestsRun=10, got %d", result.TestResults.TestsRun)
	}
}

func TestScoreToProto_AllSubObjects(t *testing.T) {
	s := &queue.Score{
		TestResults: &queue.TestResults{
			Passed:           false,
			OutputExcerpt:    "test output",
			DurationMs:       500,
			TestsRun:         5,
			TestsFailed:      2,
			FailingTestNames: []string{"TestA", "TestB"},
		},
		DiffSummary: &queue.DiffSummary{
			FilesChanged: 3,
			ChangedFiles: []string{"a.go", "b.go", "c.go"},
			LinesAdded:   42,
			LinesDeleted: 10,
			Excerpt:      "diff excerpt",
		},
		RetryHistory: &queue.RetryHistory{
			AttemptCount: 1,
			MaxRetries:   3,
			Attempts: []queue.RetryAttempt{
				{Number: 1, FailureReason: "tests failed", TimestampMs: 1000},
			},
		},
	}
	result := scoreToProto(s)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.TestResults == nil {
		t.Fatal("expected non-nil TestResults")
	}
	if result.DiffSummary == nil {
		t.Fatal("expected non-nil DiffSummary")
	}
	if result.RetryHistory == nil {
		t.Fatal("expected non-nil RetryHistory")
	}

	// Verify TestResults fields
	if result.TestResults.Passed != false {
		t.Errorf("expected Passed=false")
	}
	if result.TestResults.TestsFailed != 2 {
		t.Errorf("expected TestsFailed=2, got %d", result.TestResults.TestsFailed)
	}
	if len(result.TestResults.FailingTestNames) != 2 {
		t.Errorf("expected 2 failing test names, got %d", len(result.TestResults.FailingTestNames))
	}

	// Verify DiffSummary fields
	if result.DiffSummary.FilesChanged != 3 {
		t.Errorf("expected FilesChanged=3, got %d", result.DiffSummary.FilesChanged)
	}
	if result.DiffSummary.LinesAdded != 42 {
		t.Errorf("expected LinesAdded=42, got %d", result.DiffSummary.LinesAdded)
	}
	if result.DiffSummary.LinesDeleted != 10 {
		t.Errorf("expected LinesDeleted=10, got %d", result.DiffSummary.LinesDeleted)
	}
	if result.DiffSummary.Excerpt != "diff excerpt" {
		t.Errorf("expected Excerpt='diff excerpt', got %q", result.DiffSummary.Excerpt)
	}

	// Verify RetryHistory fields
	if result.RetryHistory.AttemptCount != 1 {
		t.Errorf("expected AttemptCount=1, got %d", result.RetryHistory.AttemptCount)
	}
	if result.RetryHistory.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", result.RetryHistory.MaxRetries)
	}
}

func TestScoreToProto_RetryHistoryTwoAttempts(t *testing.T) {
	s := &queue.Score{
		RetryHistory: &queue.RetryHistory{
			AttemptCount: 2,
			MaxRetries:   3,
			Attempts: []queue.RetryAttempt{
				{Number: 1, FailureReason: "first failure", TimestampMs: 1000},
				{Number: 2, FailureReason: "second failure", TimestampMs: 2000},
			},
		},
	}
	result := scoreToProto(s)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.RetryHistory == nil {
		t.Fatal("expected non-nil RetryHistory")
	}
	if len(result.RetryHistory.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(result.RetryHistory.Attempts))
	}

	// Verify first attempt
	a0 := result.RetryHistory.Attempts[0]
	if a0.Number != 1 {
		t.Errorf("attempt[0] Number: expected 1, got %d", a0.Number)
	}
	if a0.FailureReason != "first failure" {
		t.Errorf("attempt[0] FailureReason: expected 'first failure', got %q", a0.FailureReason)
	}
	if a0.TimestampMs != 1000 {
		t.Errorf("attempt[0] TimestampMs: expected 1000, got %d", a0.TimestampMs)
	}

	// Verify second attempt
	a1 := result.RetryHistory.Attempts[1]
	if a1.Number != 2 {
		t.Errorf("attempt[1] Number: expected 2, got %d", a1.Number)
	}
	if a1.FailureReason != "second failure" {
		t.Errorf("attempt[1] FailureReason: expected 'second failure', got %q", a1.FailureReason)
	}
	if a1.TimestampMs != 2000 {
		t.Errorf("attempt[1] TimestampMs: expected 2000, got %d", a1.TimestampMs)
	}
}

func TestScoreToProto_TestResultsPassedTrue(t *testing.T) {
	s := &queue.Score{
		TestResults: &queue.TestResults{
			Passed:      true,
			TestsRun:    5,
			TestsFailed: 0,
		},
	}
	result := scoreToProto(s)
	if result == nil || result.TestResults == nil {
		t.Fatal("expected non-nil result with TestResults")
	}
	if !result.TestResults.Passed {
		t.Errorf("expected Passed=true, got false")
	}
	if result.TestResults.TestsFailed != 0 {
		t.Errorf("expected TestsFailed=0, got %d", result.TestResults.TestsFailed)
	}
}

func TestScoreToProto_TestResultsPassedFalse(t *testing.T) {
	s := &queue.Score{
		TestResults: &queue.TestResults{
			Passed:           false,
			TestsRun:         3,
			TestsFailed:      2,
			FailingTestNames: []string{"TestX", "TestY"},
		},
	}
	result := scoreToProto(s)
	if result == nil || result.TestResults == nil {
		t.Fatal("expected non-nil result with TestResults")
	}
	if result.TestResults.Passed {
		t.Errorf("expected Passed=false, got true")
	}
	if result.TestResults.TestsFailed != 2 {
		t.Errorf("expected TestsFailed=2, got %d", result.TestResults.TestsFailed)
	}
	if len(result.TestResults.FailingTestNames) != 2 {
		t.Errorf("expected 2 failing test names, got %d", len(result.TestResults.FailingTestNames))
	}
}
