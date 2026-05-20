package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/vearne/agentscope-go/pkg/agent"
	"github.com/vearne/agentscope-go/pkg/formatter"
	"github.com/vearne/agentscope-go/pkg/memory"
	"github.com/vearne/agentscope-go/pkg/message"
	"github.com/vearne/agentscope-go/pkg/model"
)

func main() {
	apiKey := os.Getenv("AGENT_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		log.Fatal("set AGENT_API_KEY or OPENAI_API_KEY")
	}

	modelName := os.Getenv("AGENT_MODEL_NAME")
	if modelName == "" {
		modelName = "gpt-4o-mini"
	}

	fmt.Println("=== Stream Card Update Example ===")
	fmt.Println()

	m := model.NewOpenAIChatModel(modelName, apiKey, "", true)
	f := formatter.NewOpenAIChatFormatter()
	mem := memory.NewInMemoryMemory()

	ag := agent.NewDeepAgent(
		agent.WithDeepName("agent-gateway"),
		agent.WithDeepModel(m),
		agent.WithDeepFormatter(f),
		agent.WithDeepMemory(mem),
		agent.WithDeepSystemPrompt("You are a helpful assistant."),
	)

	userMsg := message.NewMsg("user", "Explain what Go channels are and give a short code example.", "user")

	fmt.Println("[Lark] SendCardReply: ⏳ 思考中... (streaming=true)")

	ch, err := ag.ReplyStream(context.Background(), userMsg)
	if err != nil {
		log.Fatalf("ReplyStream failed: %v", err)
	}

	var lastUpdate time.Time
	var lastText string
	var finalResp *message.Msg

	for streamMsg := range ch {
		finalResp = streamMsg
		text := streamMsg.GetTextContent()

		if time.Since(lastUpdate) >= 500*time.Millisecond && text != lastText {
			lastUpdate = time.Now()
			delta := text[len(lastText):]
			fmt.Printf("[Lark] UpdateCard (streaming): +%q\n", delta)
			lastText = text
		}
	}

	if finalResp != nil {
		fmt.Println("[Lark] UpdateCard (final, streaming=false):")
		fmt.Println(finalResp.GetTextContent())
	}
}
