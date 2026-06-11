package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/wsx864321/coding-agent/internal/evidence"
)

func TestCompleteStepTool_Basic(t *testing.T) {
	tool := NewCompleteStepTool()
	if tool.Name() != "complete_step" {
		t.Errorf("Name() = %q", tool.Name())
	}
}

func TestCompleteStepTool_NoLedger(t *testing.T) {
	tool := NewCompleteStepTool()
	_, err := tool.Execute(context.Background(), map[string]any{
		"step":   "step 1",
		"result": "done",
		"evidence": []any{
			map[string]any{"type": "manual", "detail": "checked manually"},
		},
	})
	if err == nil {
		t.Fatal("expected error when no ledger in context")
	}
}

func TestCompleteStepTool_ManualEvidence(t *testing.T) {
	tool := NewCompleteStepTool()
	ledger := evidence.NewLedger()
	ctx := evidence.WithLedger(context.Background(), ledger)

	out, err := tool.Execute(ctx, map[string]any{
		"step":   "step 1",
		"result": "done manually",
		"evidence": []any{
			map[string]any{"type": "manual", "detail": "I verified it works"},
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Step signed off") {
		t.Errorf("output should confirm sign-off, got %q", out)
	}
	if !ledger.IsStepCompleted("step 1") {
		t.Error("ledger should mark step as completed")
	}
}

func TestCompleteStepTool_VerificationEvidence(t *testing.T) {
	tool := NewCompleteStepTool()
	ledger := evidence.NewLedger()
	ctx := evidence.WithLedger(context.Background(), ledger)

	// 没有 bash 凭证 → verification 证据验证失败
	_, err := tool.Execute(ctx, map[string]any{
		"step":   "run tests",
		"result": "all passed",
		"evidence": []any{
			map[string]any{"type": "verification", "detail": "ran pytest"},
		},
	})
	if err == nil {
		t.Fatal("expected error without bash receipt for verification evidence")
	}
	if !strings.Contains(err.Error(), "bash") {
		t.Errorf("error = %q, should mention bash", err.Error())
	}

	// 添加 bash 凭证后重试
	ledger.Record("bash", map[string]any{"command": "pytest"}, "all passed", true)
	out, err := tool.Execute(ctx, map[string]any{
		"step":   "run tests",
		"result": "all passed",
		"evidence": []any{
			map[string]any{"type": "verification", "detail": "ran pytest"},
		},
	})
	if err != nil {
		t.Fatalf("with bash receipt: %v", err)
	}
	if !strings.Contains(out, "Step signed off") {
		t.Errorf("output = %q", out)
	}
}

func TestCompleteStepTool_DiffEvidence(t *testing.T) {
	tool := NewCompleteStepTool()
	ledger := evidence.NewLedger()
	ctx := evidence.WithLedger(context.Background(), ledger)

	// 没有写操作凭证
	_, err := tool.Execute(ctx, map[string]any{
		"step":   "edit file",
		"result": "updated",
		"evidence": []any{
			map[string]any{"type": "diff", "detail": "changed main.go"},
		},
	})
	if err == nil {
		t.Fatal("expected error without write receipt")
	}

	// 添加 edit_file 凭证
	ledger.Record("edit_file", nil, "ok", true)
	_, err = tool.Execute(ctx, map[string]any{
		"step":   "edit file",
		"result": "updated",
		"evidence": []any{
			map[string]any{"type": "diff", "detail": "changed main.go"},
		},
	})
	if err != nil {
		t.Fatalf("with edit receipt: %v", err)
	}
}

func TestCompleteStepTool_StepNotInTodos(t *testing.T) {
	tool := NewCompleteStepTool()
	ledger := evidence.NewLedger()
	ctx := evidence.WithLedger(context.Background(), ledger)

	// 设置 todo 列表
	ledger.SetTodos([]evidence.TodoItem{
		{Content: "step 1", Status: "in_progress"},
	})

	_, err := tool.Execute(ctx, map[string]any{
		"step":   "nonexistent step",
		"result": "done",
		"evidence": []any{
			map[string]any{"type": "manual", "detail": "ok"},
		},
	})
	if err == nil {
		t.Fatal("expected error for step not in todos")
	}
	if !strings.Contains(err.Error(), "不匹配") {
		t.Errorf("error = %q, should mention mismatch", err.Error())
	}
}

func TestCompleteStepTool_EmptyFields(t *testing.T) {
	tool := NewCompleteStepTool()
	ledger := evidence.NewLedger()
	ctx := evidence.WithLedger(context.Background(), ledger)

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "empty step",
			args: map[string]any{
				"step": "", "result": "ok",
				"evidence": []any{map[string]any{"type": "manual", "detail": "ok"}},
			},
			want: "step",
		},
		{
			name: "empty result",
			args: map[string]any{
				"step": "s", "result": "",
				"evidence": []any{map[string]any{"type": "manual", "detail": "ok"}},
			},
			want: "result",
		},
		{
			name: "no evidence",
			args: map[string]any{
				"step": "s", "result": "ok",
				"evidence": []any{},
			},
			want: "evidence",
		},
		{
			name: "invalid evidence type",
			args: map[string]any{
				"step": "s", "result": "ok",
				"evidence": []any{map[string]any{"type": "wrong", "detail": "ok"}},
			},
			want: "无效",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Execute(ctx, tt.args)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, should contain %q", err.Error(), tt.want)
			}
		})
	}
}

func TestCompleteStepTool_FilesEvidence(t *testing.T) {
	tool := NewCompleteStepTool()
	ledger := evidence.NewLedger()
	ctx := evidence.WithLedger(context.Background(), ledger)

	// files 类型需要读或写凭证
	_, err := tool.Execute(ctx, map[string]any{
		"step":   "check files",
		"result": "verified",
		"evidence": []any{
			map[string]any{"type": "files", "detail": "read config.json"},
		},
	})
	if err == nil {
		t.Fatal("expected error without file receipt")
	}

	// read_file 也满足 files 类型
	ledger.Record("read_file", nil, "content", true)
	_, err = tool.Execute(ctx, map[string]any{
		"step":   "check files",
		"result": "verified",
		"evidence": []any{
			map[string]any{"type": "files", "detail": "read config.json"},
		},
	})
	if err != nil {
		t.Fatalf("with read receipt: %v", err)
	}
}
