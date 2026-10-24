// Package agentwire defines the versioned HTTPS contract between server and agent.
package agentwire

type Registration struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}
type Credentials struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}
type Target struct {
	ID                string `json:"id"`
	CertificateID     string `json:"certificate_id"`
	Type              string `json:"type"`
	CertificatePath   string `json:"certificate_path"`
	KeyPath           string `json:"key_path"`
	ConfigurationPath string `json:"configuration_path"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	SNI               string `json:"sni"`
}
type Task struct {
	ID             string `json:"id"`
	TargetID       string `json:"target_id"`
	Phase          string `json:"phase"`
	Fingerprint    string `json:"fingerprint"`
	CertificatePEM string `json:"certificate_pem,omitempty"`
	KeyPEM         string `json:"key_pem,omitempty"`
}
type Result struct {
	Phase       string `json:"phase"`
	Outcome     string `json:"outcome"`
	Fingerprint string `json:"fingerprint"`
	Error       string `json:"error,omitempty"`
}
