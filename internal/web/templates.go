package web

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
)

var (
	templates    *template.Template
	tmplOnce     sync.Once
	tmplErr      error
	templatePath string
)

// initTemplates initializes the template cache
func initTemplates(logger *zap.Logger) {
	tmplOnce.Do(func() {
		// Get working directory
		workDir, err := os.Getwd()
		if err != nil {
			workDir = "."
		}
		templateDir := filepath.Join(workDir, "web", "templates")
		templatePath = templateDir

		if logger != nil {
			logger.Info("Initializing templates",
				zap.String("workDir", workDir),
				zap.String("templateDir", templateDir))
		}

		// Get all template files
		pattern := filepath.Join(templateDir, "*.html")
		files, err := filepath.Glob(pattern)
		if err != nil {
			tmplErr = fmt.Errorf("failed to glob templates: %w", err)
			if logger != nil {
				logger.Error("Template glob failed", zap.Error(tmplErr))
			}
			return
		}

		if len(files) == 0 {
			tmplErr = fmt.Errorf("no template files found in %s", templateDir)
			if logger != nil {
				logger.Error("No templates found", zap.String("pattern", pattern))
			}
			return
		}

		if logger != nil {
			logger.Info("Found template files", zap.Strings("files", files))
		}

		// Parse all templates together so they can reference each other
		// This is crucial for {{define}} and {{template}} to work across files
		templates, err = template.ParseFiles(files...)
		if err != nil {
			tmplErr = fmt.Errorf("failed to parse templates: %w", err)
			if logger != nil {
				logger.Error("Template parse failed", zap.Error(tmplErr))
			}
			return
		}

		// Log available templates
		if logger != nil && templates != nil {
			templateNames := make([]string, 0)
			for _, t := range templates.Templates() {
				templateNames = append(templateNames, t.Name())
			}
			logger.Info("Templates loaded successfully", zap.Strings("templates", templateNames))
		}
	})
}

// renderTemplate renders a template with the given data
func renderTemplate(w http.ResponseWriter, tmpl string, data interface{}, logger *zap.Logger) {
	initTemplates(logger)

	if tmplErr != nil {
		if logger != nil {
			logger.Error("Template initialization error", zap.Error(tmplErr))
		}
		http.Error(w, "Templates failed to initialize: "+tmplErr.Error(), http.StatusInternalServerError)
		return
	}

	if templates == nil {
		http.Error(w, "Templates not initialized", http.StatusInternalServerError)
		return
	}

	// List available templates for debugging
	if logger != nil {
		availableTemplates := make([]string, 0)
		for _, t := range templates.Templates() {
			availableTemplates = append(availableTemplates, t.Name())
		}
		logger.Info("Rendering template",
			zap.String("requested", tmpl),
			zap.Strings("available", availableTemplates))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, tmpl, data); err != nil {
		if logger != nil {
			logger.Error("Template execution error",
				zap.String("template", tmpl),
				zap.Error(err))
		}
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

