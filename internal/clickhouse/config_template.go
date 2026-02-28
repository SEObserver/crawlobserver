package clickhouse

import (
	"os"
	"path/filepath"
	"text/template"
)

const configXMLTemplate = `<clickhouse>
    <listen_host>127.0.0.1</listen_host>
    <tcp_port>{{.TCPPort}}</tcp_port>
    <http_port>{{.HTTPPort}}</http_port>
    <path>{{.DataDir}}/clickhouse/</path>
    <tmp_path>{{.DataDir}}/tmp/</tmp_path>
    <user_files_path>{{.DataDir}}/user_files/</user_files_path>
    <format_schema_path>{{.DataDir}}/format_schemas/</format_schema_path>
    <logger>
        <log>{{.DataDir}}/logs/clickhouse.log</log>
        <errorlog>{{.DataDir}}/logs/clickhouse.err.log</errorlog>
        <level>warning</level>
    </logger>
    <max_server_memory_usage_to_ram_ratio>0.5</max_server_memory_usage_to_ram_ratio>
    <mark_cache_size>536870912</mark_cache_size>
    <profiles>
        <default>
            <max_memory_usage>4000000000</max_memory_usage>
            <max_execution_time>60</max_execution_time>
            <max_bytes_before_external_group_by>500000000</max_bytes_before_external_group_by>
            <max_bytes_before_external_sort>500000000</max_bytes_before_external_sort>
        </default>
    </profiles>
    <users>
        <default>
            <password></password>
            <networks>
                <ip>127.0.0.1</ip>
            </networks>
            <profile>default</profile>
            <quota>default</quota>
            <access_management>0</access_management>
        </default>
    </users>
    <quotas>
        <default>
            <interval>
                <duration>3600</duration>
                <queries>0</queries>
                <errors>0</errors>
                <result_rows>0</result_rows>
                <read_rows>0</read_rows>
                <execution_time>0</execution_time>
            </interval>
        </default>
    </quotas>
</clickhouse>
`

type configTemplateData struct {
	TCPPort  int
	HTTPPort int
	DataDir  string
}

// writeConfigXML generates the ClickHouse config.xml in the data directory.
func writeConfigXML(dataDir string, tcpPort, httpPort int) (string, error) {
	// Ensure all required directories exist
	for _, sub := range []string{"clickhouse", "tmp", "user_files", "format_schemas", "logs"} {
		if err := os.MkdirAll(filepath.Join(dataDir, sub), 0755); err != nil {
			return "", err
		}
	}

	configPath := filepath.Join(dataDir, "config.xml")
	f, err := os.Create(configPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	tmpl, err := template.New("config").Parse(configXMLTemplate)
	if err != nil {
		return "", err
	}

	err = tmpl.Execute(f, configTemplateData{
		TCPPort:  tcpPort,
		HTTPPort: httpPort,
		DataDir:  dataDir,
	})
	if err != nil {
		return "", err
	}

	return configPath, nil
}
