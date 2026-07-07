package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/config"
	"github.com/bytedance/trae-agent/internal/cost"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/mcp"
	"github.com/bytedance/trae-agent/internal/memory"
	"github.com/bytedance/trae-agent/internal/permission"
	"github.com/bytedance/trae-agent/internal/session"
	"github.com/bytedance/trae-agent/internal/tool"
	"github.com/bytedance/trae-agent/internal/trajectory"
	"github.com/spf13/cobra"
)

const defaultMaxRetries = 3
const defaultRetryBaseDelay = 500 * time.Millisecond
const defaultBashTimeout = 120 * time.Second
const defaultMaxStepsFallback = 20

// buildAgent 根据 config 与 flag 构造 agent 实例，供 run / interactive 共享。
// 同时返回权限策略、权限存储、MCP 管理器，供调用方管理生命周期。
func buildAgent(ctx context.Context, cfg config.Config, providerFlag, modelFlag string) (
	*agent.Agent, *permission.DefaultPolicy, *permission.Store, *mcp.Manager, error,
) {
	providerName := providerFlag
	if providerName == "" {
		providerName = cfg.DefaultProvider
	}
	if providerName == "" {
		return nil, nil, nil, nil, fmt.Errorf("no provider specified: set default_provider in config or use --provider")
	}

	provCfg, ok := cfg.Providers[providerName]
	if !ok {
		return nil, nil, nil, nil, fmt.Errorf("provider %q not found in config", providerName)
	}

	providerType := provCfg.Provider
	if providerType == "" {
		providerType = providerName
	}
	llmProvider, err := llm.NewProvider(providerType, provCfg.APIKey, provCfg.BaseURL, provCfg.DefaultModel)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	llmProvider = llm.NewRetryable(llmProvider, defaultMaxRetries, defaultRetryBaseDelay)

	// 权限策略：从持久化存储加载
	permStore, err := permission.NewStore()
	if err != nil {
		permStore = nil
	}
	var policy *permission.DefaultPolicy
	if permStore != nil {
		policy, err = permStore.Load()
		if err != nil {
			policy = permission.NewPolicy()
		}
	} else {
		policy = permission.NewPolicy()
	}

	// MCP server：启动并发现工具
	mcpMgr := mcp.NewManager()
	if len(cfg.MCPServers) > 0 {
		mcpCfg := make(map[string]mcp.ServerConfig, len(cfg.MCPServers))
		for name, sc := range cfg.MCPServers {
			mcpCfg[name] = mcp.ServerConfig{Command: sc.Command, Args: sc.Args, Env: sc.Env}
		}
		errs := mcpMgr.StartAll(ctx, mcpCfg)
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "[mcp] %v\n", e)
		}
	}

	// 基础工具 + MCP 工具
	baseTools := []tool.Tool{
		tool.NewRead(),
		tool.NewWrite(),
		tool.NewEdit(),
		tool.NewGlob(),
		tool.NewGrep(),
		tool.NewBash(defaultBashTimeout),
		tool.NewTodo(),
	}
	baseTools = append(baseTools, mcpMgr.Tools()...)
	baseRegistry := tool.NewRegistry(baseTools...)

	subagentModel := modelFlag
	if subagentModel == "" {
		subagentModel = provCfg.DefaultModel
	}
	subagentRunner := agent.NewSubagentRunner(llmProvider, baseRegistry, subagentModel)

	fullRegistry := tool.NewRegistry(append(baseTools, tool.NewTask(subagentRunner))...)

	maxSteps := cfg.MaxSteps
	if maxSteps == 0 {
		maxSteps = defaultMaxStepsFallback
	}

	// 加载项目记忆，注入 system prompt
	workDir, _ := os.Getwd()
	mem := memory.Load(workDir)
	systemPrompt := agent.BuildSystemPrompt(mem)

	// headless 默认 AutoAsker deny；interactive 模式由 REPL 覆盖
	a := agent.New(llmProvider, fullRegistry,
		agent.WithMaxSteps(maxSteps),
		agent.WithModel(modelFlag),
		agent.WithSystemPrompt(systemPrompt),
		agent.WithPolicy(policy),
		agent.WithAsker(&permission.AutoAsker{Default: permission.ActionDeny}),
	)
	return a, policy, permStore, mcpMgr, nil
}

func NewRunCmd() *cobra.Command {
	var providerFlag, modelFlag string
	var jsonFlag, trajectoryFlag bool
	cmd := &cobra.Command{
		Use:   "run [prompt]",
		Short: "Run a prompt through the agent loop with tool access",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configFromCmd(cmd)
			if err != nil {
				return err
			}

			a, _, _, mcpMgr, err := buildAgent(cmd.Context(), cfg, providerFlag, modelFlag)
			if err != nil {
				return err
			}
			if mcpMgr != nil {
				defer mcpMgr.Close()
			}

			events := make(chan agent.Event, 64)
			errCh := make(chan error, 1)
			go func() {
				errCh <- a.Run(cmd.Context(), args[0], events)
			}()

			// --json: headless 模式
			if jsonFlag {
				return runJSON(cmd, events, errCh, modelFlag)
			}

			// --trajectory: tee 事件到轨迹记录器
			var usage llm.Usage
			var renderErr error
			if trajectoryFlag {
				rec, err := trajectory.NewRecorder(trajectory.DefaultPath(session.GenerateID()))
				if err != nil {
					fmt.Fprintf(os.Stderr, "[trajectory] init failed: %v\n", err)
				} else {
					defer rec.Close()
					usage, renderErr = renderWithTrajectory(events, cmd.OutOrStdout(), rec)
				}
			}
			if !trajectoryFlag || renderErr != nil {
				renderer := NewRenderer()
				usage, renderErr = renderer.Render(events, cmd.OutOrStdout())
			}
			if renderErr != nil {
				return renderErr
			}
			if err := <-errCh; err != nil {
				if cmd.Context().Err() != nil {
					fmt.Fprintln(os.Stderr, "\n[interrupted]")
					return nil
				}
				return err
			}
			costStr := ""
			if estCost, ok := cost.Estimate(modelFlag, usage); ok {
				costStr = fmt.Sprintf(" | cost: %s", cost.FormatCost(estCost))
			}
			fmt.Fprintf(os.Stderr, "\n[tokens: in=%d out=%d%s]\n", usage.InputTokens, usage.OutputTokens, costStr)
			return nil
		},
	}
	cmd.Flags().StringVar(&providerFlag, "provider", "", "LLM provider (anthropic/openai)")
	cmd.Flags().StringVar(&modelFlag, "model", "", "model name override")
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "Output structured JSON result (headless mode)")
	cmd.Flags().BoolVar(&trajectoryFlag, "trajectory", false, "Record trajectory to ~/.trae/trajectories/")
	return cmd
}

// jsonResult 是 --json 模式的输出结构。
type jsonResult struct {
	Text      string     `json:"text"`
	ToolCalls []jsonTool `json:"tool_calls"`
	Usage     llm.Usage  `json:"usage"`
	Cost      string     `json:"cost,omitempty"`
	Error     string     `json:"error,omitempty"`
}

type jsonTool struct {
	Name   string `json:"name"`
	Args   string `json:"args"`
	Result string `json:"result"`
}

// runJSON 收集所有事件，输出结构化 JSON。
func runJSON(cmd *cobra.Command, events <-chan agent.Event, errCh <-chan error, model string) error {
	var result jsonResult
	var tools []jsonTool
	var currentTool *jsonTool

	for ev := range events {
		switch e := ev.(type) {
		case agent.TextEvent:
			result.Text += e.Content
		case agent.ToolCallEvent:
			currentTool = &jsonTool{Name: e.Name, Args: e.Args}
		case agent.ToolResultEvent:
			if currentTool != nil {
				currentTool.Result = e.Result.Content
				tools = append(tools, *currentTool)
				currentTool = nil
			}
		case agent.DoneEvent:
			result.Usage = e.Usage
		}
	}
	result.ToolCalls = tools

	if err := <-errCh; err != nil {
		if cmd.Context().Err() != nil {
			result.Error = "interrupted"
		} else {
			result.Error = err.Error()
		}
	}
	if estCost, ok := cost.Estimate(model, result.Usage); ok {
		result.Cost = cost.FormatCost(estCost)
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// renderWithTrajectory 将事件 tee 到轨迹记录器和渲染器。
func renderWithTrajectory(events <-chan agent.Event, out io.Writer, rec *trajectory.Recorder) (llm.Usage, error) {
	tee := make(chan agent.Event, 64)
	var usage llm.Usage
	var recErr error

	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range events {
			if err := rec.RecordEvent(ev); err != nil && recErr == nil {
				recErr = err
			}
			tee <- ev
		}
		close(tee)
	}()

	renderer := NewRenderer()
	usage, renderErr := renderer.Render(tee, out)
	<-done
	if renderErr != nil {
		return usage, renderErr
	}
	return usage, recErr
}
