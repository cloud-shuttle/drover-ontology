package ingest

import (
	"strings"
	"testing"
)

func TestCodeAnalysis_ParseFileSymbols_JSTS(t *testing.T) {
	jsContent := `import { something } from 'somewhere';

// This is a comment
class BannerGenerator extends BaseGenerator {
  constructor(options) {
    super(options);
    this.options = options;
  }

  async generate(speakerName) {
    console.log("Generating banner for", speakerName);
    return { success: true };
  }
}

const renderBanner = (options) => {
  const innerHelper = () => {
    return true;
  };
  return innerHelper();
}

export function oldSchoolFunction(a, b) {
  return a + b;
}`

	// Construct our CodeAnalysis context
	context := "=== FILE: src/banner.js ===\n" + jsContent + "\n\n"
	analysis := &CodeAnalysis{Context: context}

	res, err := analysis.ParseFileSymbols("src/banner.js")
	if err != nil {
		t.Fatalf("ParseFileSymbols failed: %v", err)
	}

	t.Logf("Result for JS:\n%s", res)

	if !strings.Contains(res, "BannerGenerator [lines 4-14]") {
		t.Errorf("Expected BannerGenerator block line spans [lines 4-14]")
	}
	if !strings.Contains(res, "generate [lines 10-13]") {
		t.Errorf("Expected class method generate block line spans [lines 10-13]")
	}
	if !strings.Contains(res, "renderBanner [lines 16-21]") {
		t.Errorf("Expected arrow function assignment renderBanner block line spans [lines 16-21]")
	}
	if !strings.Contains(res, "oldSchoolFunction [lines 23-25]") {
		t.Errorf("Expected export function oldSchoolFunction block line spans [lines 23-25]")
	}
}

func TestCodeAnalysis_ParseFileSymbols_Python(t *testing.T) {
	pyContent := `import os

class MeetupBannerService:
    """A service for banners."""
    def __init__(self, theme):
        self.theme = theme

    def build_canvas(self, width, height):
        # build canvas helper
        return canvas

def make_banner(name):
    service = MeetupBannerService("dark")
    return service.build_canvas(800, 600)`

	context := "=== FILE: banner_service.py ===\n" + pyContent + "\n\n"
	analysis := &CodeAnalysis{Context: context}

	res, err := analysis.ParseFileSymbols("banner_service.py")
	if err != nil {
		t.Fatalf("ParseFileSymbols failed: %v", err)
	}

	t.Logf("Result for Python:\n%s", res)

	if !strings.Contains(res, "MeetupBannerService [lines 3-11]") {
		t.Errorf("Expected Python class MeetupBannerService block line spans [lines 3-11]")
	}
	if !strings.Contains(res, "__init__ [lines 5-7]") {
		t.Errorf("Expected python def __init__ block line spans [lines 5-7]")
	}
	if !strings.Contains(res, "build_canvas [lines 8-11]") {
		t.Errorf("Expected python def build_canvas block line spans [lines 8-11]")
	}
	if !strings.Contains(res, "make_banner [lines 12-14]") {
		t.Errorf("Expected python top level def make_banner block line spans [lines 12-14]")
	}
}

func TestCodeAnalysis_ParseFileSymbols_YAML(t *testing.T) {
	yamlContent := `project: typescript-banners
version: 1.0.0
dependencies:
  - lodash
  - canvas

# Configuration settings
settings:
  theme: dark
  dimensions:
    width: 1200
    height: 630`

	context := "=== FILE: config.yaml ===\n" + yamlContent + "\n\n"
	analysis := &CodeAnalysis{Context: context}

	res, err := analysis.ParseFileSymbols("config.yaml")
	if err != nil {
		t.Fatalf("ParseFileSymbols failed: %v", err)
	}

	t.Logf("Result for YAML:\n%s", res)

	if !strings.Contains(res, "project [lines 1-1]") {
		t.Errorf("Expected YAML top-level key project spans [lines 1-1]")
	}
	if !strings.Contains(res, "dependencies [lines 3-7]") {
		t.Errorf("Expected YAML top-level key dependencies spans [lines 3-7]")
	}
	if !strings.Contains(res, "settings [lines 8-12]") {
		t.Errorf("Expected YAML top-level key settings spans [lines 8-12]")
	}
}

func TestCodeAnalysis_ParseFileSymbols_Fallback(t *testing.T) {
	txtContent := `This is a standard text file
with no syntax highlighting.

struct Config {
  title: string
}

fn buildConfig() {
  return
}`

	context := "=== FILE: readme.txt ===\n" + txtContent + "\n\n"
	analysis := &CodeAnalysis{Context: context}

	res, err := analysis.ParseFileSymbols("readme.txt")
	if err != nil {
		t.Fatalf("ParseFileSymbols failed: %v", err)
	}

	t.Logf("Result for Text Fallback:\n%s", res)

	if !strings.Contains(res, "Config (struct) [line 4]") {
		t.Errorf("Expected fallback structural signature match for struct Config")
	}
	if !strings.Contains(res, "buildConfig (fn) [line 8]") {
		t.Errorf("Expected fallback structural signature match for fn buildConfig")
	}
}
