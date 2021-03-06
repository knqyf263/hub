package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/artifacthub/hub/internal/hub"
)

// Scanner describes the methods a Scanner implementation must provide.
type Scanner interface {
	Scan(image string) ([]byte, error)
}

// ScanSnapshot scans the provided package's snapshot for security
// vulnerabilities returning a report with the results.
func ScanSnapshot(
	ctx context.Context,
	scanner Scanner,
	snapshot *hub.SnapshotToScan,
	ec hub.ErrorsCollector,
) (*hub.SnapshotSecurityReport, error) {
	full := make(map[string][]interface{})
	ec.Init(snapshot.RepositoryID)

	for _, image := range snapshot.ContainersImages {
		parts := strings.Split(image.Image, ":")
		if len(parts) == 1 || parts[1] == "latest" {
			ec.Append(snapshot.RepositoryID, fmt.Sprintf("latest tag not supported: %s", image.Image))
			continue
		}
		reportData, err := scanner.Scan(image.Image)
		if err != nil {
			if errors.Is(err, ErrImageNotFound) {
				ec.Append(snapshot.RepositoryID, fmt.Sprintf("%s: %s", err.Error(), image.Image))
				continue
			}
			err := fmt.Errorf("error scanning image %s: %w", image.Image, err)
			ec.Append(snapshot.RepositoryID, err.Error())
			return nil, err
		}
		var imageFullReport []interface{}
		if err := json.Unmarshal(reportData, &imageFullReport); err != nil {
			return nil, fmt.Errorf("error unmarshalling image %s full report: %w", image.Image, err)
		}
		if imageFullReport != nil {
			full[image.Image] = imageFullReport
		}
	}
	var summary *hub.SecurityReportSummary
	if len(full) > 0 {
		summary = generateSummary(full)
	} else {
		full = nil
	}

	return &hub.SnapshotSecurityReport{
		PackageID: snapshot.PackageID,
		Version:   snapshot.Version,
		Full:      full,
		Summary:   summary,
	}, nil
}

// generateSummary generates a summary of the security report from the full
// report
func generateSummary(full map[string][]interface{}) *hub.SecurityReportSummary {
	summary := &hub.SecurityReportSummary{}
	for _, targets := range full {
		for _, entry := range targets {
			var target *Target
			entryJSON, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			if err := json.Unmarshal(entryJSON, &target); err != nil {
				continue
			}
			for _, vulnerability := range target.Vulnerabilities {
				switch vulnerability.Severity {
				case "CRITICAL":
					summary.Critical++
				case "HIGH":
					summary.High++
				case "MEDIUM":
					summary.Medium++
				case "LOW":
					summary.Low++
				case "UNKNOWN":
					summary.Unknown++
				}
			}
		}
	}
	return summary
}

// Target represents a target in a security report.
type Target struct {
	Vulnerabilities []*Vulnerability `json:"Vulnerabilities"`
}

// Vulnerability represents a vulnerability in a security report target.
type Vulnerability struct {
	Severity string `json:"Severity"`
}
