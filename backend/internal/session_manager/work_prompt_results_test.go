package sessionmanager

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

func TestWorkOriginalsSectionNamesResultsInsideOriginal(t *testing.T) {
	source := filepath.Join(t.TempDir(), "06_Archive")
	results := filepath.Join(source, "Output")
	project := domain.ProjectRecord{Kind: domain.ProjectKindWork}
	project.Config.Work = &domain.WorkConfig{Sources: []string{source}, ResultsDir: results}
	got := workOriginalsSection(project, results)
	if !strings.Contains(got, "skip it when you read or list the originals") {
		t.Fatalf("section does not mention the results folder inside the original:\n%s", got)
	}
	project.Config.Work.ResultsDir = filepath.Join(t.TempDir(), "elsewhere")
	if strings.Contains(workOriginalsSection(project, results), "skip it") {
		t.Fatal("results outside the originals should not get the note")
	}
}
