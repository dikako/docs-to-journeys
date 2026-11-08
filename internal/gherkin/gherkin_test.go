package gherkin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dikako/docs-to-journeys/internal/common/claude"
)

const feature = `Feature: Checkout

  Background:
    Given I am logged in as "standard_user"

  Scenario: Add backpack to cart
    When I tap "Sauce Labs Backpack"
    And I tap "Add to cart"
    Then the cart badge shows "1"

  Scenario Outline: Login fails
    When I type "<user>" in the Username field
    Then I see "<error>"
    And I stay on the Login screen

    Examples:
      | user   | error            |
      | locked | user is locked   |
      | nobody | invalid username |
`

func TestParseExpandsBackgroundAndOutline(t *testing.T) {
	scs, err := Parse([]byte(feature), "f.feature")
	if err != nil {
		t.Fatal(err)
	}
	if len(scs) != 3 {
		t.Fatalf("want 3 scenarios, got %d", len(scs))
	}
	if scs[0].Steps[0].Text != `I am logged in as "standard_user"` {
		t.Fatalf("background not prepended: %+v", scs[0].Steps)
	}
	if scs[0].Outcomes() != 1 || scs[1].Outcomes() != 2 {
		t.Fatalf("outcomes: %d %d", scs[0].Outcomes(), scs[1].Outcomes())
	}
	if !strings.Contains(scs[2].Steps[1].Text, "nobody") {
		t.Fatalf("outline not substituted: %+v", scs[2].Steps)
	}
	if scs[1].Slug != "login-fails-1" || scs[2].Slug != "login-fails-2" {
		t.Fatalf("outline rows must be numbered: %q %q", scs[1].Slug, scs[2].Slug)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse([]byte("not gherkin at all\n"), "x"); err == nil {
		t.Fatal("want parse error")
	}
	if _, err := Parse([]byte("Feature: empty\n"), "x"); err == nil {
		t.Fatal("want error for no scenarios")
	}
}

type scriptedAgent struct{ replies []string }

func (a *scriptedAgent) Run(_ context.Context, _ claude.Prompt) (claude.Result, error) {
	r := a.replies[0]
	a.replies = a.replies[1:]
	return claude.Result{Reply: r}, nil
}

func TestToJourneysRetriesOnMissingAssertion(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "checkout.feature")
	_ = os.WriteFile(fp, []byte(feature), 0o644)
	good1 := `{"description":"d","actions":["Log in","Tap \"Sauce Labs Backpack\"","Tap \"Add to cart\"","Verify the cart badge shows \"1\""]}`
	bad := `{"description":"d","actions":["Log in","Type \"locked\" in the Username field","Verify \"user is locked\" is shown"]}`
	good2 := `here: {"description":"d","actions":["Log in","Type \"locked\" in the Username field","Verify \"user is locked\" is shown","Verify the Login screen is still shown"]}`
	good3 := `{"description":"d","actions":["Log in","Type \"nobody\" in the Username field","Verify \"invalid username\" is shown","Verify the Login screen is still shown"]}`
	ag := &scriptedAgent{replies: []string{good1, bad, good2, good3}}
	s := NewGherkinService(Deps{Agent: ag, JourneyDir: filepath.Join(dir, "gen"), Pkg: "p"})

	res, err := s.ToJourneys(context.Background(), fp)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Err != nil {
			t.Fatalf("%s: %v", r.Path, r.Err)
		}
	}
	if len(ag.replies) != 0 {
		t.Fatalf("unused replies: %d", len(ag.replies))
	}
	// Second run keeps existing (possibly healed) journeys.
	res, _ = s.ToJourneys(context.Background(), fp)
	for _, r := range res {
		if !r.Skipped {
			t.Fatalf("%s should be skipped", r.Path)
		}
	}
}
