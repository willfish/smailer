package main

import (
	"strings"
	"testing"
)

func TestCutToWidth_PlainASCII(t *testing.T) {
	prefix, remainder := cutToWidth("hello world", 5)
	if prefix != "hello" {
		t.Errorf("prefix = %q, want %q", prefix, "hello")
	}
	if remainder != " world" {
		t.Errorf("remainder = %q, want %q", remainder, " world")
	}
}

func TestCutToWidth_ExactFit(t *testing.T) {
	prefix, remainder := cutToWidth("hello", 5)
	if prefix != "hello" {
		t.Errorf("prefix = %q, want %q", prefix, "hello")
	}
	if remainder != "" {
		t.Errorf("remainder = %q, want %q", remainder, "")
	}
}

func TestCutToWidth_ZeroWidth(t *testing.T) {
	prefix, remainder := cutToWidth("hello", 0)
	if prefix != "" {
		t.Errorf("prefix = %q, want %q", prefix, "")
	}
	if remainder != "hello" {
		t.Errorf("remainder = %q, want %q", remainder, "hello")
	}
}

func TestCutToWidth_EmptyString(t *testing.T) {
	prefix, remainder := cutToWidth("", 5)
	if prefix != "" {
		t.Errorf("prefix = %q, want %q", prefix, "")
	}
	if remainder != "" {
		t.Errorf("remainder = %q, want %q", remainder, "")
	}
}

func TestCutToWidth_ANSIEscapesPreserved(t *testing.T) {
	// ANSI escape for red text: \033[31m
	input := "\033[31mhello\033[0m world"
	prefix, _ := cutToWidth(input, 5)
	// Should include the opening escape and all 5 visible characters
	if !strings.Contains(prefix, "\033[31m") {
		t.Errorf("prefix should contain ANSI escape, got %q", prefix)
	}
	if !strings.Contains(prefix, "hello") {
		t.Errorf("prefix should contain 'hello', got %q", prefix)
	}
}

func TestCutToWidth_ANSIDoesNotCountAsWidth(t *testing.T) {
	// The ANSI escape has zero visible width, so cutting at width 3
	// should give us the escape + "hel"
	input := "\033[31mhello"
	prefix, remainder := cutToWidth(input, 3)
	if prefix != "\033[31mhel" {
		t.Errorf("prefix = %q, want %q", prefix, "\033[31mhel")
	}
	if remainder != "lo" {
		t.Errorf("remainder = %q, want %q", remainder, "lo")
	}
}

func TestPlaceOverlay_CentredOnBase(t *testing.T) {
	base := strings.Join([]string{
		"aaaaaaaaaa",
		"aaaaaaaaaa",
		"aaaaaaaaaa",
		"aaaaaaaaaa",
	}, "\n")

	overlay := "XX"

	result := placeOverlay(4, 1, overlay, base)
	lines := strings.Split(result, "\n")

	if lines[0] != "aaaaaaaaaa" {
		t.Errorf("line 0 should be unchanged, got %q", lines[0])
	}
	// Line 1 should have overlay at position 4
	if !strings.Contains(lines[1], "XX") {
		t.Errorf("line 1 should contain overlay, got %q", lines[1])
	}
	if lines[2] != "aaaaaaaaaa" {
		t.Errorf("line 2 should be unchanged, got %q", lines[2])
	}
}

func TestPlaceOverlay_AtOrigin(t *testing.T) {
	base := "abcdef\nghijkl"
	overlay := "XY"

	result := placeOverlay(0, 0, overlay, base)
	lines := strings.Split(result, "\n")

	if !strings.HasPrefix(lines[0], "XY") {
		t.Errorf("line 0 should start with overlay, got %q", lines[0])
	}
}

func TestPlaceOverlay_BeyondBase(t *testing.T) {
	base := "abc"
	overlay := "XY\nZW"

	// Overlay starts at y=0 but extends beyond base (only 1 line)
	result := placeOverlay(0, 0, overlay, base)
	lines := strings.Split(result, "\n")

	// Should only have 1 line (base has 1 line, second overlay line is out of bounds)
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d", len(lines))
	}
}
