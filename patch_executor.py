import re

with open("internal/orchestrator/executor.go", "r") as f:
    code = f.read()

code = code.replace("func ExecuteCommand(deviceID string, cmd string) error {", "func ExecuteCommand(schema, deviceID string, cmd string) error {")
code = code.replace("""	err := database.DB.QueryRow("SELECT last_ip FROM devices WHERE id = $1", deviceID).Scan(&targetIP)""", """	err := database.DB.QueryRow(fmt.Sprintf("SELECT last_ip FROM %s.devices WHERE id = $1", schema), deviceID).Scan(&targetIP)""")

with open("internal/orchestrator/executor.go", "w") as f:
    f.write(code)

with open("internal/services/shaping_manager.go", "r") as f:
    code = f.read()

code = code.replace("err := orchestrator.ExecuteCommand(deviceID, cmd)", "err := orchestrator.ExecuteCommand(schema, deviceID, cmd)")

with open("internal/services/shaping_manager.go", "w") as f:
    f.write(code)

