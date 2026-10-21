package service

import "testing"

func TestValidateCanvasApplyOpsResultAcceptsAdjustedPositions(t *testing.T) {
	x, y := 100.0, 200.0
	input := canvasApplyOpsArguments{Ops: []canvasApplyOp{
		{Op: "add_text", Ref: "a", Text: "开场", X: &x, Y: &y},
		{Op: "move", NodeID: "existing", X: &x, Y: &y},
		{Op: "connect", From: "a", To: "existing"},
	}}
	actualX, actualY := 432.0, 200.0
	far := 1e12
	build := func() *SubmitAgentToolResultRequest {
		return &SubmitAgentToolResultRequest{
			NodeIDs: []string{"text-1", "existing"},
			Ops: []AgentToolOpResult{
				{Op: "add_text", Ref: "a", NodeID: "text-1", X: &actualX, Y: &actualY, Adjusted: true},
				{Op: "move", NodeID: "existing", X: &actualX, Y: &actualY, Adjusted: true},
				{Op: "connect", FromNodeID: "text-1", ToNodeID: "existing"},
			},
		}
	}
	if err := validateCanvasApplyOpsResult(input, build()); err != nil {
		t.Fatalf("adjusted result rejected: %v", err)
	}
	bad := build()
	bad.Ops[2].Adjusted, bad.Ops[2].X, bad.Ops[2].Y = true, &actualX, &actualY
	if validateCanvasApplyOpsResult(input, bad) == nil {
		t.Fatal("connect must not carry coordinates")
	}
	bad = build()
	bad.Ops[0].X = &far
	if validateCanvasApplyOpsResult(input, bad) == nil {
		t.Fatal("out-of-range coordinates must be rejected")
	}
	bad = build()
	bad.Ops[1].Adjusted = false
	if validateCanvasApplyOpsResult(input, bad) == nil {
		t.Fatal("coordinates without adjusted flag must be rejected")
	}
}
