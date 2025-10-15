package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/chzyer/readline"
	"github.com/fatih/color"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/ssestream"
)

func main() {
	modelName := os.Getenv("MODEL_NAME")
	reasoningModel := os.Getenv("REASONING_MODEL") == "true"

	// OpenAI API Client
	client := openai.NewClient()
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{},
		Seed:     openai.Int(0),
		Model:    modelName,
	}

	if err := checkServer(client, params); err != nil {
		err = fmt.Errorf("%v\n\nUnable to chat. Make sure the server has started successfully.\n", err)
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	// Make the llm believe it told us it's an assistant. System messages are ignored?
	params.Messages = append(params.Messages, openai.AssistantMessage("How may I assist you today?"))

	fmt.Printf("Connected to %v\n", os.Getenv("OPENAI_BASE_URL"))
	fmt.Println("Type your prompt, then ENTER to submit. CTRL-C to quit.")

	rl, err := readline.NewEx(&readline.Config{
		Prompt: color.RedString("» "),
		//HistoryFile:     "/tmp/readline.tmp",
		//AutoComplete:    completer,
		InterruptPrompt: "^C",
		//EOFPrompt:       "exit", // Does not work as expected

		HistorySearchFold:   true,
		FuncFilterInputRune: filterInput,
	})
	if err != nil {
		log.Fatalf("Can't init readline: %v", err)
	}
	defer rl.Close()
	//rl.CaptureExitSignal() // Should readline capture and handle the exit signal? - Can be used to interrupt the chat response stream.
	log.SetOutput(rl.Stderr())

	for {
		prompt, err := rl.Readline()
		if errors.Is(err, readline.ErrInterrupt) {
			if len(prompt) == 0 {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}
		if prompt == "exit" {
			break
		}

		params = handlePrompt(client, params, reasoningModel, prompt)
	}
	fmt.Println("Closing chat")
}

func checkServer(client openai.Client, params openai.ChatCompletionNewParams) error {
	params.Messages = []openai.ChatCompletionMessageParamUnion{openai.SystemMessage("You are a helpful assistant")}

	stopProgress := startProgressSpinner("Connecting to " + os.Getenv("OPENAI_BASE_URL"))
	defer stopProgress()

	timeoutContext, timeoutCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer timeoutCancel()

	_, err := client.Chat.Completions.New(timeoutContext, params)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// Connecting to server failed: context deadline exceeded
			// Taking longer than the timeout means the LLM is thinking
			return nil
		} else {
			// Post "http://server:8080/v3/chat/completions": dial tcp 192.168.86.81:8080: connect: connection refused
			// POST "http://server:8080/v3/chat/completions": 404 Not Found "Mediapipe graph definition with requested name is not found"
			return err
		}
	}

	return nil
}

func handlePrompt(client openai.Client, params openai.ChatCompletionNewParams, reasoningModel bool, prompt string) openai.ChatCompletionNewParams {
	params.Messages = append(params.Messages, openai.UserMessage(prompt))

	paramDebugString, _ := json.Marshal(params)

	if os.Getenv("DEBUG") == "true" {
		log.Printf("Sending request:\n%s", paramDebugString)
	}

	stream := client.Chat.Completions.NewStreaming(context.Background(), params)
	appendParam := processStream(stream, reasoningModel)

	// Store previous prompts for context
	if appendParam != nil {
		params.Messages = append(params.Messages, *appendParam)
	}
	fmt.Println()
	fmt.Println()

	return params
}

func processStream(stream *ssestream.Stream[openai.ChatCompletionChunk], printThinking bool) *openai.ChatCompletionMessageParamUnion {

	// optionally, an accumulator helper can be used
	acc := openai.ChatCompletionAccumulator{}

	// For reasoning models we assume the first output is them thinking, because the opening <think> tag is not always present.
	thinking := printThinking

	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		if _, ok := acc.JustFinishedContent(); ok {
			//fmt.Println("\nContent stream finished")
		}

		// if using tool calls
		if tool, ok := acc.JustFinishedToolCall(); ok {
			fmt.Printf("Tool call stream finished %d: %s %s", tool.Index, tool.Name, tool.Arguments)
		}

		if refusal, ok := acc.JustFinishedRefusal(); ok {
			fmt.Printf("Refusal stream finished: %s", refusal)
		}

		// Print chunks as they are received
		if len(chunk.Choices) > 0 {
			lastChunk := chunk.Choices[0].Delta.Content

			if strings.Contains(lastChunk, "<think>") {
				thinking = true
				fmt.Printf("%s", color.BlueString(lastChunk))
			} else if strings.Contains(lastChunk, "</think>") {
				thinking = false
				fmt.Printf("%s", color.BlueString(lastChunk))

			} else if thinking {
				fmt.Printf("%s", color.BlueString(lastChunk))

			} else {
				fmt.Printf("%s", lastChunk)
			}
		}
	}

	if stream.Err() != nil {
		err := fmt.Errorf("\n\nError reading response stream: %v\n", stream.Err())
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}

	// After the stream is finished, acc can be used like a ChatCompletion
	appendParam := acc.Choices[0].Message.ToParam()
	if acc.Choices[0].Message.Content == "" {
		return nil
	}
	return &appendParam
}

func filterInput(r rune) (rune, bool) {
	switch r {
	// block CtrlZ feature
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}

func startProgressSpinner(prefix string) (stop func()) {
	s := spinner.New(spinner.CharSets[9], time.Millisecond*200)
	s.Prefix = prefix
	s.Start()

	return s.Stop
}
