/*
 */
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/ssestream"

	"github.com/chzyer/readline"
)

func main() {
	modelName := os.Getenv("MODEL_NAME")
	reasoningModel := os.Getenv("REASONING_MODEL") == "True"

	// OpenAI API Client
	client := openai.NewClient()
	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{},
		Seed:     openai.Int(0),
		Model:    modelName,
	}

	if err := checkServer(client, params); err != nil {
		log.Fatalf("Connecting to server failed: %v", err)
	}

	fmt.Println("Type your prompt, then ENTER to submit. CTRL-C to quit.")

	rl, err := readline.NewEx(&readline.Config{
		Prompt: Red + "» " + ColorOff,
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

	stream := client.Chat.Completions.NewStreaming(context.Background(), params)
	appendParam := processStream(stream, reasoningModel)

	// Store previous prompts for context
	params.Messages = append(params.Messages, appendParam)
	fmt.Println()
	fmt.Println()

	return params
}

func processStream(stream *ssestream.Stream[openai.ChatCompletionChunk], printThinking bool) openai.ChatCompletionMessageParamUnion {

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

		// Print chunks as they are received. Colors from: https://gist.github.com/vratiu/9780109
		if len(chunk.Choices) > 0 {
			lastChunk := chunk.Choices[0].Delta.Content

			if strings.Contains(lastChunk, "<think>") {
				thinking = true
				fmt.Printf(Purple+"%s"+ColorOff, lastChunk)
			} else if strings.Contains(lastChunk, "</think>") {
				thinking = false
				fmt.Printf(Purple+"%s"+ColorOff, lastChunk)

			} else if thinking {
				fmt.Printf(Purple+"%s"+ColorOff, lastChunk)

			} else {
				fmt.Printf(Blue+"%s"+ColorOff, lastChunk)
			}
		}
	}

	if stream.Err() != nil {
		panic(stream.Err())
	}

	// After the stream is finished, acc can be used like a ChatCompletion
	return acc.Choices[0].Message.ToParam()
}

func filterInput(r rune) (rune, bool) {
	switch r {
	// block CtrlZ feature
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}
