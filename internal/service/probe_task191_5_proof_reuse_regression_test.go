package service

import (
	"context"
	"testing"
	"task191-reproof/internal/model"
)

func TestBug05_IdenticalEvidenceReusesProof(t *testing.T) {
	app, _ := openApp(t)
	ctx := context.Background()
	tid := buildDemoTarget(t, app)
	acts, _ := app.ListActions(ctx, tid)
	for _, action := range acts {
		if action.Name == "b" {
			if _, err := app.AddDeclaration(ctx, action.ID, "env.local", model.DirRead, ""); err != nil { t.Fatal(err) }
		}
	}
	if _, _, err := app.AnalyzeTarget(ctx, tid); err != nil { t.Fatal(err) }
	p1, err := app.GenerateProof(ctx, tid)
	if err != nil { t.Fatal(err) }
	p2, err := app.GenerateProof(ctx, tid)
	if err != nil { t.Fatal(err) }
	if p1.ID != p2.ID { t.Fatalf("identical evidence must reuse proof: %d != %d", p1.ID, p2.ID) }
}
