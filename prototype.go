package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PrototypeService struct {
	defaultExportDir string
}

func NewPrototypeService(defaultExportDir string) *PrototypeService {
	return &PrototypeService{defaultExportDir: defaultExportDir}
}

func (s *PrototypeService) BuildBundle(project Project, schema UISchema, renderSpec PrototypeRenderBundleSpec) PrototypeBundle {
	projectSlug := slugify(project.Name)
	renderSpec = normalizePrototypeRenderBundleSpec(renderSpec)
	if strings.TrimSpace(renderSpec.AppTSX) == "" {
		renderSpec.AppTSX = prototypeAppTSX()
	}
	if strings.TrimSpace(renderSpec.StylesCSS) == "" {
		renderSpec.StylesCSS = prototypeStylesCSS()
	}
	if strings.TrimSpace(renderSpec.PreviewHTML) == "" {
		renderSpec.PreviewHTML = prototypePreviewHTML(project, schema)
	}
	files := []GeneratedFile{
		{Path: "package.json", Contents: prototypePackageJSON(project.Name)},
		{Path: "src/main.tsx", Contents: prototypeMainTSX()},
		{Path: "src/App.tsx", Contents: renderSpec.AppTSX},
		{Path: "src/schema.ts", Contents: "export const schema = " + mustJSON(schema) + " as const;\n"},
		{Path: "src/styles.css", Contents: renderSpec.StylesCSS},
		{Path: "preview.html", Contents: renderSpec.PreviewHTML},
		{Path: "README.md", Contents: prototypeReadme(project.Name)},
	}
	return PrototypeBundle{
		ProjectID:     project.ID,
		ProjectName:   project.Name,
		EntryFile:     filepath.ToSlash(filepath.Join(projectSlug, "src", "App.tsx")),
		GeneratedAt:   nowUTC(),
		UISchema:      schema,
		RenderMode:    renderSpec.RenderMode,
		DesignSummary: renderSpec.DesignSummary,
		PreviewHTML:   renderSpec.PreviewHTML,
		Files:         files,
	}
}

func (s *PrototypeService) ExportBundle(project Project, schema UISchema, renderSpec PrototypeRenderBundleSpec, outputDir string) (PrototypeBundle, error) {
	bundle := s.BuildBundle(project, schema, renderSpec)
	targetDir := strings.TrimSpace(outputDir)
	if targetDir == "" {
		targetDir = filepath.Join(s.defaultExportDir, slugify(project.Name))
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return PrototypeBundle{}, err
	}
	for _, file := range bundle.Files {
		fullPath := filepath.Join(targetDir, file.Path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return PrototypeBundle{}, err
		}
		if err := os.WriteFile(fullPath, []byte(file.Contents), 0o644); err != nil {
			return PrototypeBundle{}, err
		}
	}
	bundle.ExportPath = targetDir
	return bundle, nil
}

func prototypePackageJSON(projectName string) string {
	return fmt.Sprintf(`{
  "name": "%s-prototype",
  "private": true,
  "version": "0.0.1",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1"
  },
  "devDependencies": {
    "@types/react": "^18.3.5",
    "@types/react-dom": "^18.3.0",
    "@vitejs/plugin-react": "^4.3.1",
    "typescript": "^5.6.3",
    "vite": "^5.4.8"
  }
}
`, slugify(projectName))
}

func prototypeMainTSX() string {
	return `import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./styles.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
`
}

func prototypeAppTSX() string {
	return `import { useMemo, useState } from "react";
import { schema } from "./schema";

type Route = typeof schema.routes[number];
type Page = typeof schema.pages[number];

function App() {
  const [currentPath, setCurrentPath] = useState(schema.routes[0]?.path ?? "/");
  const currentRoute = schema.routes.find((route) => route.path === currentPath) ?? schema.routes[0];
  const currentPage = useMemo<Page | undefined>(
    () => schema.pages.find((page) => page.id === currentRoute?.page_id),
    [currentRoute]
  );

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div>
          <p className="eyebrow">Prototype</p>
          <h1>{schema.app_meta.name}</h1>
          <p className="description">{schema.app_meta.description}</p>
        </div>
        <nav className="nav">
          {schema.routes.map((route: Route) => (
            <button
              key={route.path}
              className={route.path === currentPath ? "nav-item active" : "nav-item"}
              onClick={() => setCurrentPath(route.path)}
            >
              {route.label}
            </button>
          ))}
        </nav>
      </aside>
      <main className="canvas">
        {!currentPage ? (
          <section className="page-card">No page configured.</section>
        ) : (
          <section className="page-card">
            <p className="eyebrow">{currentPage.type}</p>
            <h2>{currentPage.title}</h2>
            <p>{currentPage.description}</p>
            <div className="chip-row">
              {currentPage.primary_ctas.map((item: string) => (
                <span key={item} className="chip primary">{item}</span>
              ))}
              {currentPage.secondary_ctas.map((item: string) => (
                <span key={item} className="chip">{item}</span>
              ))}
            </div>
            <div className="block-grid">
              {currentPage.components.map((componentId: string) => {
                const component = schema.components.find((item) => item.id === componentId);
                return (
                  <article key={componentId} className="block">
                    <h3>{component?.label ?? componentId}</h3>
                    <p>{component?.description ?? "Component placeholder"}</p>
                  </article>
                );
              })}
            </div>
          </section>
        )}
      </main>
    </div>
  );
}

export default App;
`
}

func prototypeStylesCSS() string {
	return `:root {
  color-scheme: light;
  font-family: "Space Grotesk", "Segoe UI", sans-serif;
  background: #f6f1e8;
  color: #1b1e24;
}

* {
  box-sizing: border-box;
}

body {
  margin: 0;
  min-height: 100vh;
  background:
    radial-gradient(circle at top left, rgba(232, 140, 91, 0.18), transparent 32%),
    linear-gradient(180deg, #f8f4ea 0%, #efe5d6 100%);
}

#root {
  min-height: 100vh;
}

.app-shell {
  min-height: 100vh;
  display: grid;
  grid-template-columns: 320px 1fr;
}

.sidebar {
  padding: 32px;
  background: rgba(20, 28, 39, 0.96);
  color: #f7efe4;
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.eyebrow {
  margin: 0 0 10px;
  text-transform: uppercase;
  letter-spacing: 0.12em;
  font-size: 12px;
  opacity: 0.72;
}

.description {
  line-height: 1.6;
  color: rgba(247, 239, 228, 0.78);
}

.nav {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.nav-item {
  border: 1px solid rgba(247, 239, 228, 0.15);
  background: rgba(255, 255, 255, 0.06);
  color: inherit;
  border-radius: 16px;
  padding: 14px 16px;
  text-align: left;
  cursor: pointer;
}

.nav-item.active {
  background: linear-gradient(135deg, #f0915f 0%, #f6c58c 100%);
  color: #1c1c20;
  border-color: transparent;
}

.canvas {
  padding: 32px;
}

.page-card {
  min-height: calc(100vh - 64px);
  background: rgba(255, 252, 247, 0.92);
  border-radius: 28px;
  padding: 28px;
  box-shadow: 0 18px 50px rgba(34, 30, 25, 0.12);
}

.chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin: 18px 0 24px;
}

.chip {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 8px 12px;
  background: #ede4d6;
}

.chip.primary {
  background: #1f2937;
  color: #fff;
}

.block-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
}

.block {
  padding: 18px;
  border-radius: 20px;
  background: #fff;
  border: 1px solid rgba(27, 30, 36, 0.08);
}

@media (max-width: 860px) {
  .app-shell {
    grid-template-columns: 1fr;
  }
}
`
}

func prototypeReadme(projectName string) string {
	return fmt.Sprintf("# %s Prototype\n\nThis folder contains an AI-generated React prototype bundle.\n\n- `src/App.tsx`: generated application shell\n- `src/styles.css`: generated visual styles\n- `src/schema.ts`: UISchema source of truth\n- `preview.html`: standalone preview page for quick review\n", projectName)
}
