package ssl

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// expiringWindow is how close to expiry a certificate is reported as expiring.
const expiringWindow = 30 * 24 * time.Hour

// certInfo is what Abstrax reads from a certificate on disk.
type certInfo struct {
	NotAfter time.Time
	Domains  []string
}

// readCertificate parses the leaf certificate in a certbot live directory.
func readCertificate(letsEncryptDir, certName string) (*certInfo, error) {
	live := filepath.Join(letsEncryptDir, "live", certName)
	var lastErr error
	for _, file := range []string{"cert.pem", "fullchain.pem"} {
		data, err := os.ReadFile(filepath.Join(live, file))
		if err != nil {
			lastErr = err
			continue
		}
		info, err := parseCertificatePEM(data)
		if err != nil {
			lastErr = err
			continue
		}
		return info, nil
	}
	return nil, lastErr
}

func parseCertificatePEM(data []byte) (*certInfo, error) {
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("no certificate found in PEM data")
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		domains := append([]string(nil), cert.DNSNames...)
		if len(domains) == 0 && cert.Subject.CommonName != "" {
			domains = []string{cert.Subject.CommonName}
		}
		return &certInfo{NotAfter: cert.NotAfter.UTC(), Domains: domains}, nil
	}
}

// classify maps an expiry to a status and whole days remaining (negative when expired).
func classify(now, notAfter time.Time) (status string, days int) {
	remaining := notAfter.Sub(now)
	days = int(remaining.Hours() / 24)
	switch {
	case remaining <= 0:
		return StatusExpired, days
	case remaining < expiringWindow:
		return StatusExpiring, days
	default:
		return StatusValid, days
	}
}

// parseCertbotExpiry reads the date from "Expiry Date: 2026-12-01 10:00:00+00:00 (VALID: 29 days)".
func parseCertbotExpiry(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, " ("); i >= 0 {
		raw = raw[:i]
	}
	for _, layout := range []string{"2006-01-02 15:04:05-07:00", "2006-01-02 15:04:05Z07:00", time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// scrapedCert is one certificate parsed from human-readable `certbot certificates` output.
type scrapedCert struct {
	Name     string
	Domains  []string
	NotAfter time.Time
	HasDate  bool
}

// parseCertbotCertificates is the fallback for certificates Abstrax cannot read
// directly (different permissions, non-standard layout, certs no project owns).
func parseCertbotCertificates(out string) []scrapedCert {
	var certs []scrapedCert
	var cur *scrapedCert
	flush := func() {
		if cur != nil {
			certs = append(certs, *cur)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Certificate Name:"):
			flush()
			cur = &scrapedCert{Name: strings.TrimSpace(strings.TrimPrefix(line, "Certificate Name:"))}
		case cur != nil && strings.HasPrefix(line, "Domains:"):
			cur.Domains = strings.Fields(strings.TrimPrefix(line, "Domains:"))
		case cur != nil && strings.HasPrefix(line, "Expiry Date:"):
			if t, ok := parseCertbotExpiry(strings.TrimPrefix(line, "Expiry Date:")); ok {
				cur.NotAfter, cur.HasDate = t, true
			}
		}
	}
	flush()
	return certs
}
