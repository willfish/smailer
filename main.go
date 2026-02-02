package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	bucket := os.Getenv("BUCKET")
	prefix := os.Getenv("PREFIX")
	if prefix == "" {
		prefix = "inbound/"
	}
	prefix = strings.TrimRight(prefix, "/") + "/"

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = os.Getenv("REGION")
	}
	if region == "" {
		region = "eu-west-2"
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		fmt.Printf("Error loading AWS config: %v\n", err)
		os.Exit(1)
	}
	client := s3.NewFromConfig(cfg)

	p := tea.NewProgram(initialModel(client, bucket, prefix))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func initialModel(client *s3.Client, bucket, prefix string) model {
	s := spinner.New()
	s.Spinner = spinner.Globe
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := model{
		s3Client: client,
		bucket:   bucket,
		prefix:   prefix,
		hasMore:  true,
		spinner:  s,
		loading:  true,
	}

	if bucket == "" {
		m.state = bucketSelectionState
	} else {
		m.state = listState
	}

	return m
}
