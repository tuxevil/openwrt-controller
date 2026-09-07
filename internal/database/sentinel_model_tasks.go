package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type SentinelModelTask struct {
	ID             string
	CaseID         string
	ReasoningClass string
	Prompt         string
	Status         string
	Result         string
	Attempts       int
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func sentinelModelTasksSchema(schema string) (string, error) {
	safe, err := SafeSchemaIdent(schema)
	if err != nil {
		return "", err
	}
	return pgx.Identifier{safe}.Sanitize(), nil
}

func EnsureSentinelModelTaskTable(schema string) error {
	quoted, err := sentinelModelTasksSchema(schema)
	if err != nil {
		return err
	}
	_, err = DB.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.sentinel_model_tasks (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		case_id UUID NOT NULL REFERENCES %s.sentinel_cases(id) ON DELETE CASCADE,
		reasoning_class VARCHAR(32) NOT NULL,
		prompt TEXT NOT NULL,
		status VARCHAR(20) NOT NULL DEFAULT 'QUEUED',
		result TEXT NOT NULL DEFAULT '',
		attempts INT NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		not_before TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CHECK (reasoning_class IN ('curation','frontier_escalation')),
		CHECK (status IN ('QUEUED','RUNNING','COMPLETED','FAILED'))
	)`, quoted, quoted))
	if err != nil {
		return err
	}
	if _, err = DB.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_sentinel_model_tasks_ready
		ON %s.sentinel_model_tasks(status, not_before, created_at)`, quoted)); err != nil {
		return err
	}
	_, err = DB.Exec(fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS idx_sentinel_model_tasks_active_unique
		ON %s.sentinel_model_tasks(case_id, reasoning_class)
		WHERE status IN ('QUEUED','RUNNING')`, quoted))
	return err
}

func QueueSentinelModelTask(schema, caseID, reasoningClass, prompt string) (SentinelModelTask, error) {
	if err := EnsureSentinelModelTaskTable(schema); err != nil {
		return SentinelModelTask{}, err
	}
	quoted, err := sentinelModelTasksSchema(schema)
	if err != nil {
		return SentinelModelTask{}, err
	}
	var task SentinelModelTask
	query := fmt.Sprintf(`INSERT INTO %s.sentinel_model_tasks (case_id, reasoning_class, prompt)
		VALUES ($1::uuid, $2, $3)
		ON CONFLICT (case_id, reasoning_class) WHERE status IN ('QUEUED','RUNNING') DO NOTHING
		RETURNING id::text, case_id::text, reasoning_class, prompt, status, result, attempts, last_error, created_at, updated_at`, quoted)
	err = DB.QueryRow(query, caseID, reasoningClass, prompt).Scan(
		&task.ID, &task.CaseID, &task.ReasoningClass, &task.Prompt, &task.Status, &task.Result,
		&task.Attempts, &task.LastError, &task.CreatedAt, &task.UpdatedAt,
	)
	if err == nil {
		return task, nil
	}
	if err != sql.ErrNoRows {
		return SentinelModelTask{}, err
	}
	query = fmt.Sprintf(`SELECT id::text, case_id::text, reasoning_class, prompt, status, result, attempts, last_error, created_at, updated_at
		FROM %s.sentinel_model_tasks
		WHERE case_id = $1::uuid AND reasoning_class = $2 AND status IN ('QUEUED','RUNNING')
		ORDER BY created_at DESC LIMIT 1`, quoted)
	err = DB.QueryRow(query, caseID, reasoningClass).Scan(
		&task.ID, &task.CaseID, &task.ReasoningClass, &task.Prompt, &task.Status, &task.Result,
		&task.Attempts, &task.LastError, &task.CreatedAt, &task.UpdatedAt,
	)
	return task, err
}

func ClaimSentinelModelTask(schema string) (SentinelModelTask, error) {
	if err := EnsureSentinelModelTaskTable(schema); err != nil {
		return SentinelModelTask{}, err
	}
	quoted, err := sentinelModelTasksSchema(schema)
	if err != nil {
		return SentinelModelTask{}, err
	}
	var task SentinelModelTask
	query := fmt.Sprintf(`WITH next_task AS (
		SELECT id FROM %s.sentinel_model_tasks
		WHERE status = 'QUEUED' AND not_before <= CURRENT_TIMESTAMP
		ORDER BY created_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	)
	UPDATE %s.sentinel_model_tasks t
	SET status = 'RUNNING', attempts = attempts + 1, updated_at = CURRENT_TIMESTAMP
	FROM next_task
	WHERE t.id = next_task.id
	RETURNING t.id::text, t.case_id::text, t.reasoning_class, t.prompt, t.status, t.result, t.attempts, t.last_error, t.created_at, t.updated_at`, quoted, quoted)
	err = DB.QueryRow(query).Scan(
		&task.ID, &task.CaseID, &task.ReasoningClass, &task.Prompt, &task.Status, &task.Result,
		&task.Attempts, &task.LastError, &task.CreatedAt, &task.UpdatedAt,
	)
	return task, err
}

func CompleteSentinelModelTask(schema, taskID, result string) error {
	quoted, err := sentinelModelTasksSchema(schema)
	if err != nil {
		return err
	}
	res, err := DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_model_tasks
		SET status = 'COMPLETED', result = $1, last_error = '', updated_at = CURRENT_TIMESTAMP
		WHERE id = $2::uuid AND status = 'RUNNING'`, quoted), result, taskID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("Sentinel model task %s was not running", taskID)
	}
	return nil
}

func RequeueSentinelModelTask(schema, taskID string, retryAt time.Time, lastError string) error {
	quoted, err := sentinelModelTasksSchema(schema)
	if err != nil {
		return err
	}
	res, err := DB.Exec(fmt.Sprintf(`UPDATE %s.sentinel_model_tasks
		SET status = 'QUEUED', last_error = $1, not_before = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3::uuid AND status = 'RUNNING'`, quoted), lastError, retryAt, taskID)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("Sentinel model task %s was not running", taskID)
	}
	return nil
}
