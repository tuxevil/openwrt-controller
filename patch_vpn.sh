cat << 'OUTER'
*** Begin Patch
*** Update File: internal/services/vpn.go
@@ -30,6 +30,9 @@
-// AssignInternalIP assigns a unique 10.8.0.x IP for the given device ID
-func AssignInternalIP(deviceID string) (string, error) {
-	var wgIP sql.NullString
-	err := database.DB.QueryRow("SELECT wg_ip FROM devices WHERE id = $1", deviceID).Scan(&wgIP)
+func AssignInternalIP(schema string, deviceID string) (string, error) {
+	if schema == "" {
+		schema = "public"
+	}
+	var wgIP sql.NullString
+	err := database.DB.QueryRow(fmt.Sprintf("SELECT wg_ip FROM %s.devices WHERE id = $1", schema), deviceID).Scan(&wgIP)
 	if err != nil && err != sql.ErrNoRows {
 		return "", err
 	}
@@ -52,3 +55,3 @@
-	rows, err := tx.Query("SELECT wg_ip FROM devices WHERE wg_ip IS NOT NULL AND wg_ip LIKE '10.8.0.%'")
+	rows, err := tx.Query(fmt.Sprintf("SELECT wg_ip FROM %s.devices WHERE wg_ip IS NOT NULL AND wg_ip LIKE '10.8.0.%%'", schema))
 	if err != nil {
 		return "", err
 	}
@@ -79,3 +82,3 @@
-	_, err = tx.Exec("UPDATE devices SET wg_ip = $1 WHERE id = $2", newIP, deviceID)
+	_, err = tx.Exec(fmt.Sprintf("UPDATE %s.devices SET wg_ip = $1 WHERE id = $2", schema), newIP, deviceID)
 	if err != nil {
 		return "", err
 	}
*** End Patch
OUTER
