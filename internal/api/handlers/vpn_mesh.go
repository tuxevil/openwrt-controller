package handlers

import (
	"encoding/json"
	"net/http"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"openwrt-controller/internal/database"
	"openwrt-controller/internal/models"
	"openwrt-controller/internal/orchestrator"
)

func GetVPNMeshesHandler(w http.ResponseWriter, r *http.Request) {
	schema := r.Context().Value("schema").(string)
	meshes, err := database.GetVPNMeshes(schema)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if meshes == nil {
		meshes = []models.VPNMesh{}
	}
	json.NewEncoder(w).Encode(meshes)
}

func CreateVPNMeshHandler(w http.ResponseWriter, r *http.Request) {
	schema := r.Context().Value("schema").(string)
	var mesh models.VPNMesh
	if err := json.NewDecoder(r.Body).Decode(&mesh); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	if mesh.Topology == "" {
		mesh.Topology = "hub_and_spoke"
	}
	if mesh.Subnet == "" {
		mesh.Subnet = "10.9.0.0/24"
	}

	if err := database.CreateVPNMesh(schema, &mesh); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(mesh)
}

func DeleteVPNMeshHandler(w http.ResponseWriter, r *http.Request) {
	schema := r.Context().Value("schema").(string)
	meshID := r.PathValue("mesh_id")

	if err := database.DeleteVPNMesh(schema, meshID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func GetVPNMeshNodesHandler(w http.ResponseWriter, r *http.Request) {
	schema := r.Context().Value("schema").(string)
	meshID := r.PathValue("mesh_id")

	nodes, err := database.GetVPNMeshNodes(schema, meshID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if nodes == nil {
		nodes = []models.VPNMeshNode{}
	}
	json.NewEncoder(w).Encode(nodes)
}

func AddVPNMeshNodeHandler(w http.ResponseWriter, r *http.Request) {
	schema := r.Context().Value("schema").(string)
	meshID := r.PathValue("mesh_id")

	var req struct {
		DeviceID   string `json:"device_id"`
		Role       string `json:"role"`
		InternalIP string `json:"internal_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	priv, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		http.Error(w, "failed to generate key", http.StatusInternalServerError)
		return
	}

	node := &models.VPNMeshNode{
		MeshID:     meshID,
		DeviceID:   req.DeviceID,
		Role:       req.Role,
		InternalIP: req.InternalIP,
		PrivateKey: priv.String(),
		PublicKey:  priv.PublicKey().String(),
		ListenPort: 51821, // default
	}

	if err := database.AddVPNMeshNode(schema, node); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(node)
}

func DeleteVPNMeshNodeHandler(w http.ResponseWriter, r *http.Request) {
	schema := r.Context().Value("schema").(string)
	nodeID := r.PathValue("node_id")

	if err := database.DeleteVPNMeshNode(schema, nodeID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func SyncVPNMeshHandler(w http.ResponseWriter, r *http.Request) {
	schema := r.Context().Value("schema").(string)
	meshID := r.PathValue("mesh_id")

	// Call orchestrator
	// In order to avoid circular dependencies, we either put orchestrator logic there or import it.
	// The problem is orchestrator uses database, and database is in internal/database.
	// I'll leave the sync logic to a service.
	// But let's just pretend we call a simple func. Let's create an HTTP endpoint for it.
	if err := orchestrator.SyncVPNMesh(schema, meshID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
