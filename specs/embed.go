// Package specs встраивает SDD-спецификации (OpenSpec) в бинарник бэкенда.
// YAML-файлы этой директории — источник истины: по ним строятся промпт агента,
// реестр MCP-инструментов и матрица RBAC (см. backend/internal/spec).
package specs

import "embed"

// FS содержит все *.openspec.yaml файлы спецификаций.
//
//go:embed *.openspec.yaml
var FS embed.FS

// Имена файлов спецификаций.
const (
	AgentBehaviorFile = "01_agent_behavior.openspec.yaml"
	MCPToolsFile      = "02_mcp_tools.openspec.yaml"
	RBACMatrixFile    = "03_rbac_matrix.openspec.yaml"
)
