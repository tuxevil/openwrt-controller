package database

import (
	"fmt"
	"database/sql"
	"openwrt-controller/internal/models"
)

func CreateVPNMesh(schema string, mesh *models.VPNMesh) error {
	query := fmt.Sprintf(`
		INSERT INTO %s.vpn_meshes (name, topology, hub_device_id, subnet)
		VALUES ($1, $2, $3, $4) RETURNING id, created_at
	`, schema)
	return DB.QueryRow(query, mesh.Name, mesh.Topology, mesh.HubDeviceID, mesh.Subnet).
		Scan(&mesh.ID, &mesh.CreatedAt)
}

func GetVPNMeshes(schema string) ([]models.VPNMesh, error) {
	query := fmt.Sprintf(`SELECT id, name, topology, hub_device_id, subnet, created_at FROM %s.vpn_meshes`, schema)
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var meshes []models.VPNMesh
	for rows.Next() {
		var m models.VPNMesh
		var hub sql.NullString
		if err := rows.Scan(&m.ID, &m.Name, &m.Topology, &hub, &m.Subnet, &m.CreatedAt); err != nil {
			return nil, err
		}
		if hub.Valid {
			m.HubDeviceID = hub.String
		}
		meshes = append(meshes, m)
	}
	return meshes, nil
}

func GetVPNMeshNodes(schema string, meshID string) ([]models.VPNMeshNode, error) {
	query := fmt.Sprintf(`
		SELECT n.id, n.mesh_id, n.device_id, d.name, n.role, n.private_key, n.public_key, n.listen_port, n.internal_ip, n.created_at
		FROM %s.vpn_mesh_nodes n
		LEFT JOIN %s.devices d ON n.device_id = d.id
		WHERE n.mesh_id = $1
	`, schema, schema)
	rows, err := DB.Query(query, meshID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []models.VPNMeshNode
	for rows.Next() {
		var n models.VPNMeshNode
		var dName sql.NullString
		if err := rows.Scan(&n.ID, &n.MeshID, &n.DeviceID, &dName, &n.Role, &n.PrivateKey, &n.PublicKey, &n.ListenPort, &n.InternalIP, &n.CreatedAt); err != nil {
			return nil, err
		}
		if dName.Valid {
			n.DeviceName = dName.String
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func AddVPNMeshNode(schema string, node *models.VPNMeshNode) error {
	query := fmt.Sprintf(`
		INSERT INTO %s.vpn_mesh_nodes (mesh_id, device_id, role, private_key, public_key, listen_port, internal_ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at
	`, schema)
	return DB.QueryRow(query, node.MeshID, node.DeviceID, node.Role, node.PrivateKey, node.PublicKey, node.ListenPort, node.InternalIP).
		Scan(&node.ID, &node.CreatedAt)
}

func DeleteVPNMesh(schema string, meshID string) error {
	_, err := DB.Exec(fmt.Sprintf(`DELETE FROM %s.vpn_meshes WHERE id = $1`, schema), meshID)
	return err
}

func DeleteVPNMeshNode(schema string, nodeID string) error {
	_, err := DB.Exec(fmt.Sprintf(`DELETE FROM %s.vpn_mesh_nodes WHERE id = $1`, schema), nodeID)
	return err
}
