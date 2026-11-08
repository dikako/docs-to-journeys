package appium

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var actions = []string{
	`Tap "Hamburger" menu`,
	`Tap "Log In" menu`,
	`Type "standard_user" in the Username field`,
	`Tap the "LOGIN" button`,
	`Scroll down to "Sauce Labs Onesie"`,
	`Verify that the "PRODUCTS" screen is shown`,
}

const stepsJSON = `[
 {"step":1,"action":"tap","element":"view menu","by":"accessibilityId","value":"View menu"},
 {"step":2,"action":"tap","element":"LOGIN_MENU_ITEM","by":"accessibilityId","value":"Login Menu Item"},
 {"step":3,"action":"type","input":"standard_user \"q\"","element":"USERNAME_FIELD","by":"id","value":"com.app:id/nameET"},
 {"step":4,"action":"tap","element":"LOGIN_BUTTON","by":"accessibilityId","value":"Tap to login"},
 {"step":5,"action":"scroll","element":"ONESIE","by":"accessibilityId","value":"Sauce Labs Onesie"},
 {"step":6,"action":"verify","element":"TITLE","by":"id","value":"com.app:id/productTV"},
 {"step":6,"action":"verify","element":"TITLE","by":"id","value":"com.app:id/other"},
 {"step":9,"action":"tap","element":"OUT_OF_RANGE","by":"id","value":"x"},
 {"step":2,"action":"teleport","element":"BAD_BY","by":"css","value":".x"}
]`

func decode(t *testing.T) Script {
	t.Helper()
	s, err := Decode([]byte(stepsJSON), "Login with standard user", "login", actions, 0)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDecodeValidates(t *testing.T) {
	s := decode(t)
	if len(s.Steps) != 8 {
		t.Fatalf("want 8 steps (out-of-range dropped), got %d", len(s.Steps))
	}
	if s.Steps[0].Locator.Name != "VIEW_MENU" {
		t.Fatalf("name not normalized: %q", s.Steps[0].Locator.Name)
	}
	if s.Steps[6].Locator.Name != "TITLE_2" {
		t.Fatalf("clashing name not made unique: %q", s.Steps[6].Locator.Name)
	}
	if last := s.Steps[7]; last.Action != Other || !last.Locator.IsZero() {
		t.Fatalf("unknown action/strategy not sanitized: %+v", last)
	}
	if got := len(s.Locators()); got != 7 {
		t.Fatalf("want 7 distinct locators, got %d", got)
	}
}

func TestJavaRender(t *testing.T) {
	e, err := Lookup("java")
	if err != nil {
		t.Fatal(err)
	}
	s := decode(t)
	if e.FileName(s) != "LoginJourney.java" {
		t.Fatalf("file name %q", e.FileName(s))
	}
	src := string(e.Render(s))
	for _, want := range []string{
		`public static final By VIEW_MENU = AppiumBy.accessibilityId("View menu");`,
		`public static final By USERNAME_FIELD = AppiumBy.id("com.app:id/nameET");`,
		`// 3. Type "standard_user" in the Username field`,
		`visible(wait, USERNAME_FIELD).sendKeys("standard_user \"q\"");`,
		`visible(wait, LOGIN_BUTTON).click();`,
		`scrollIntoView(new UiSelector().description(\"Sauce Labs Onesie\"))`,
		`assertTrue(visible(wait, TITLE).isDisplayed(), "Verify that the \"PRODUCTS\" screen is shown");`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing:\n  %s\nin:\n%s", want, src)
		}
	}
	if strings.Count(src, "// 6. ") != 1 {
		t.Error("an action with several steps should get one comment")
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, err := Lookup("cobol"); err == nil || !strings.Contains(err.Error(), "java") {
		t.Fatalf("want error listing supported languages, got %v", err)
	}
}

// TestJavaCompiles compiles the output against minimal stubs of the Appium,
// Selenium and JUnit APIs it uses. Skipped when javac is not installed.
func TestJavaCompiles(t *testing.T) {
	if _, err := exec.LookPath("javac"); err != nil {
		t.Skip("javac not installed")
	}
	dir := t.TempDir()
	stubs := map[string]string{
		"io/appium/java_client/AppiumBy.java":                    "package io.appium.java_client; public class AppiumBy extends org.openqa.selenium.By { public static org.openqa.selenium.By accessibilityId(String s){return null;} public static org.openqa.selenium.By id(String s){return null;} public static org.openqa.selenium.By androidUIAutomator(String s){return null;} public static org.openqa.selenium.By xpath(String s){return null;} }",
		"io/appium/java_client/android/AndroidDriver.java":       "package io.appium.java_client.android; public class AndroidDriver implements org.openqa.selenium.WebDriver { public org.openqa.selenium.WebElement findElement(org.openqa.selenium.By b){return null;} public Navigation navigate(){return null;} }",
		"org/openqa/selenium/By.java":                            "package org.openqa.selenium; public class By {}",
		"org/openqa/selenium/WebElement.java":                    "package org.openqa.selenium; public interface WebElement { void click(); void clear(); void sendKeys(CharSequence... k); boolean isDisplayed(); }",
		"org/openqa/selenium/WebDriver.java":                     "package org.openqa.selenium; public interface WebDriver { WebElement findElement(By b); Navigation navigate(); interface Navigation { void back(); } }",
		"org/openqa/selenium/support/ui/WebDriverWait.java":      "package org.openqa.selenium.support.ui; public class WebDriverWait { public WebDriverWait(org.openqa.selenium.WebDriver d, java.time.Duration t){} public <T> T until(java.util.function.Function<org.openqa.selenium.WebDriver,T> f){return null;} }",
		"org/openqa/selenium/support/ui/ExpectedConditions.java": "package org.openqa.selenium.support.ui; public class ExpectedConditions { public static java.util.function.Function<org.openqa.selenium.WebDriver,org.openqa.selenium.WebElement> visibilityOfElementLocated(org.openqa.selenium.By b){return null;} }",
		"org/junit/jupiter/api/Assertions.java":                  "package org.junit.jupiter.api; public class Assertions { public static void assertTrue(boolean b, String m){} }",
	}
	var files []string
	for p, src := range stubs {
		full := filepath.Join(dir, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte(src), 0o644)
		files = append(files, full)
	}
	s := decode(t)
	e, _ := Lookup("java")
	gen := filepath.Join(dir, e.FileName(s))
	_ = os.WriteFile(gen, e.Render(s), 0o644)
	files = append(files, gen)

	out, err := exec.Command("javac", append([]string{"-d", filepath.Join(dir, "out")}, files...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("javac failed: %v\n%s\n--- source ---\n%s", err, out, e.Render(s))
	}
}

func TestDecodeDropsTapBeforeType(t *testing.T) {
	data := `[{"step":1,"action":"tap","element":"F","by":"id","value":"a:id/f"},
	          {"step":1,"action":"type","input":"x","element":"F","by":"id","value":"a:id/f"}]`
	s, err := Decode([]byte(data), "j", "j", []string{"Type x in F"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Steps) != 1 || s.Steps[0].Action != Type {
		t.Fatalf("want only the type step, got %+v", s.Steps)
	}
}
