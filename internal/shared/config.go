package shared

type ServerConfig struct {
	Server ServerSettings `yaml:"server"`
	Auth   AuthSettings   `yaml:"auth"`
	TLS    TLSSettings    `yaml:"tls"`
}

type ServerSettings struct {
	HTTPPort           int    `yaml:"http_port"`
	HTTPSPort          int    `yaml:"https_port"`
	BaseDomain         string `yaml:"base_domain"`
	AdminPort          int    `yaml:"admin_port"`
	MaxTunnelsPerToken int    `yaml:"max_tunnels_per_token"`
	MaxTotalTunnels    int    `yaml:"max_total_tunnels"`
}

type AuthSettings struct {
	Tokens []string `yaml:"tokens"`
}

type TLSSettings struct {
	AutoCert     bool   `yaml:"autocert"`
	CertCacheDir string `yaml:"cert_cache_dir"`
	CertFile     string `yaml:"cert_file,omitempty"`
	KeyFile      string `yaml:"key_file,omitempty"`
}

type ClientConfig struct {
	Server       string `json:"server"`
	Token        string `json:"token"`
	LocalPort    int    `json:"local_port"`
	Subdomain    string `json:"subdomain,omitempty"`
	CustomDomain string `json:"custom_domain,omitempty"`
	TLS          bool   `json:"tls"`
}
