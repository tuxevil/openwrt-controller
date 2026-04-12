package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// Estructura raíz usando mapas dinámicos para tolerar la variación entre chips
type PayloadCompleto struct {
	DeviceID   string                 `json:"device_id"`
	Board      map[string]interface{} `json:"board"`
	System     map[string]interface{} `json:"system"`
	Devices    map[string]interface{} `json:"devices"`
	Interfaces map[string]interface{} `json:"interfaces"`
	Wireless   map[string]interface{} `json:"wireless"`
	DHCP       map[string]interface{} `json:"dhcp"`
}

func handleTelemetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	// 1. Leer todo el body en crudo primero
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error leyendo body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 2. Decodificar para extraer estadísticas rápidas
	var telemetria PayloadCompleto
	if err := json.Unmarshal(bodyBytes, &telemetria); err != nil {
		fmt.Println("Error decodificando:", err)
		return
	}

	// 3. Guardar el JSON crudo en un archivo para que puedas analizarlo
	filename := fmt.Sprintf("dump_%s.json", telemetria.DeviceID)
	// Formatear con sangría para que sea legible
	var prettyJSON map[string]interface{}
	json.Unmarshal(bodyBytes, &prettyJSON)
	prettyBytes, _ := json.MarshalIndent(prettyJSON, "", "    ")
	
	err = os.WriteFile(filename, prettyBytes, 0644)
	if err == nil {
		fmt.Printf("💾 Payload completo guardado en: %s\n", filename)
	}

	// 4. Extraer estadísticas rápidas para la consola
	modelo := "Desconocido"
	if m, ok := telemetria.Board["model"].(string); ok {
		modelo = m
	}

	clientes := 0
	if leases, ok := telemetria.DHCP["leases"].([]interface{}); ok {
		clientes = len(leases)
	}

	fmt.Printf("📡 Router: %s | Modelo: %s\n", telemetria.DeviceID, modelo)
	fmt.Printf("👥 Clientes DHCP conectados: %d\n", clientes)
	fmt.Printf("🔌 Interfaces físicas reportadas: %d\n", len(telemetria.Devices))
	fmt.Println("---------------------------------------------------")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "ok"}`))
}

func main() {
	http.HandleFunc("/api/telemetry", handleTelemetry)
	fmt.Println("Controlador en modo captura total iniciado en el puerto 3000...")
	http.ListenAndServe(":3000", nil)
}
