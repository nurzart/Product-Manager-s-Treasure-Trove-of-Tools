package main

import (
	"errors"
	"fmt"
	"strings"
)

func (d ArtifactDocument) Validate() error {
	if strings.TrimSpace(d.Title) == "" {
		return errors.New("document title is required")
	}
	if len(d.Sections) == 0 {
		return errors.New("document requires at least one section")
	}
	for _, section := range d.Sections {
		if strings.TrimSpace(section.ID) == "" {
			return fmt.Errorf("section id is required")
		}
		if strings.TrimSpace(section.Title) == "" {
			return fmt.Errorf("section title is required")
		}
	}
	return nil
}

func (s UISchema) Validate() error {
	if strings.TrimSpace(s.AppMeta.Name) == "" {
		return errors.New("ui schema app_meta.name is required")
	}
	if len(s.Routes) == 0 {
		return errors.New("ui schema requires at least one route")
	}
	if len(s.Pages) == 0 {
		return errors.New("ui schema requires at least one page")
	}
	pageIDs := map[string]struct{}{}
	for _, page := range s.Pages {
		if strings.TrimSpace(page.ID) == "" {
			return fmt.Errorf("ui page id is required")
		}
		pageIDs[page.ID] = struct{}{}
	}
	for _, route := range s.Routes {
		if strings.TrimSpace(route.Path) == "" || strings.TrimSpace(route.PageID) == "" {
			return fmt.Errorf("ui routes require path and page_id")
		}
		if _, ok := pageIDs[route.PageID]; !ok {
			return fmt.Errorf("route %s references unknown page %s", route.Path, route.PageID)
		}
	}
	return nil
}
