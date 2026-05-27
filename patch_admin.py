import os

filepath = 'internal/database/postgres.go'
with open(filepath, 'r') as f:
    content = f.read()

# I want to replace the hardcoded "REPLACE_WITH_BOOTSTRAP_PASSWORD" with a generated one
# and print it prominently.

old_block = """		hash, err := bcrypt.GenerateFromPassword([]byte("REPLACE_WITH_BOOTSTRAP_PASSWORD"), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash bootstrap password: %w", err)
		}
		_, err = DB.Exec(
			"INSERT INTO users (username, password_hash, role) VALUES ($1, $2, 'SUPERADMIN')",
			"admin", string(hash),
		)
		if err != nil {
			return fmt.Errorf("failed to seed superadmin user: %w", err)
		}
		log.Println("Bootstrap SUPERADMIN user created (username: admin)")"""

new_block = """		adminPass := os.Getenv("SUPERADMIN_DEFAULT_PASSWORD")
		if adminPass == "" {
			b := make([]byte, 12)
			rand.Read(b)
			adminPass = hex.EncodeToString(b)
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("failed to hash bootstrap password: %w", err)
		}
		_, err = DB.Exec(
			"INSERT INTO users (username, password_hash, role) VALUES ($1, $2, 'SUPERADMIN')",
			"admin", string(hash),
		)
		if err != nil {
			return fmt.Errorf("failed to seed superadmin user: %w", err)
		}
		log.Println("=========================================================")
		log.Println("Bootstrap SUPERADMIN user created!")
		log.Println("Username: admin")
		log.Printf("Password: %s\\n", adminPass)
		log.Println("Please change this password immediately after login.")
		log.Println("=========================================================")"""

content = content.replace(old_block, new_block)

with open(filepath, 'w') as f:
    f.write(content)
