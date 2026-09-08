#!/bin/sh

# === CONFIGURACIÓN ===
# The signed artifact is immutable. Runtime credentials and the controller
# endpoint live in a root-owned file written by the image bootstrap.
NERVE_CONFIG_FILE="${NERVE_CONFIG_FILE:-/etc/nerve/agent.conf}"
if [ -r "$NERVE_CONFIG_FILE" ]; then
    . "$NERVE_CONFIG_FILE"
fi
CONTROLLER_URL="${CONTROLLER_URL:-}"
REQUIRE_TLS="${REQUIRE_TLS:-false}"
CONTROLLER_CA_FILE="${CONTROLLER_CA_FILE:-}"
CONTROLLER_PINNED_PUBKEY="${CONTROLLER_PINNED_PUBKEY:-}"
CONTROLLER_URL="${CONTROLLER_URL%/}"
BASE_URL="$CONTROLLER_URL"
TELEMETRY_URL="$BASE_URL/telemetry"
DEVICE_ID_FILE="${DEVICE_ID_FILE:-/etc/nerve-device-id}"
DEVICE_ID="$(cat "$DEVICE_ID_FILE" 2>/dev/null || true)"
DEVICE_TOKEN_FILE="${DEVICE_TOKEN_FILE:-/etc/nerve-device-token}"
DEVICE_TOKEN="$(cat "$DEVICE_TOKEN_FILE" 2>/dev/null || true)"
ENROLLMENT_TOKEN_FILE="${ENROLLMENT_TOKEN_FILE:-/etc/nerve/enrollment-token}"
ENROLLMENT_NONCE_FILE="${ENROLLMENT_NONCE_FILE:-/etc/nerve/enrollment-nonce}"
AGENT_UPDATE_PUBLIC_KEY_FILE="${AGENT_UPDATE_PUBLIC_KEY_FILE:-/etc/nerve/agent-update-public-key}"
AGENT_UPDATE_PUBLIC_KEY="$(cat "$AGENT_UPDATE_PUBLIC_KEY_FILE" 2>/dev/null || true)"
AGENT_VERSION_NUMBER_FILE="${AGENT_VERSION_NUMBER_FILE:-/etc/nerve/agent-version-number}"
AGENT_VERSION_NUMBER="$(cat "$AGENT_VERSION_NUMBER_FILE" 2>/dev/null || printf '0')"
case "$AGENT_VERSION_NUMBER" in ''|*[!0-9]*) AGENT_VERSION_NUMBER=0 ;; esac
NERVE_TRANSACTION_ROOT="${NERVE_TRANSACTION_ROOT:-/etc/nerve/transactions}"
NERVE_CONFIG_ROOT="${NERVE_CONFIG_ROOT:-/etc/config}"
NERVE_WIFI_HASH_FILE="${NERVE_WIFI_HASH_FILE:-$NERVE_TRANSACTION_ROOT/wifi_config.hash}"
NERVE_WG_HASH_FILE="${NERVE_WG_HASH_FILE:-$NERVE_TRANSACTION_ROOT/wg_config.hash}"
NERVE_OPERATION_STATUS_FILE="${NERVE_OPERATION_STATUS_FILE:-$NERVE_TRANSACTION_ROOT/operation_status}"
NERVE_CHANGE_SET_STATUS_FILE="${NERVE_CHANGE_SET_STATUS_FILE:-$NERVE_TRANSACTION_ROOT/change_set_status}"

# Keep telemetry independent from optional full GNU coreutils packages.
join_csv() {
    awk 'BEGIN { first = 1 } { if (!first) printf ","; printf "%s", $0; first = 0 } END { if (!first) printf "\n" }'
}

decode_base64() {
    if command -v base64 >/dev/null 2>&1; then
        base64 -d
    elif command -v openssl >/dev/null 2>&1; then
        openssl base64 -d -A
    else
        return 1
    fi
}

tls_required() {
    case "$REQUIRE_TLS" in
        1|true|TRUE|yes|YES) return 0 ;;
        0|false|FALSE|no|NO) return 1 ;;
        *) return 2 ;;
    esac
}

validate_controller_url() {
    case "$REQUIRE_TLS" in
        1|true|TRUE|yes|YES|0|false|FALSE|no|NO) ;;
        *) return 1 ;;
    esac
    case "$CONTROLLER_URL" in
        https://*|http://*) ;;
        *) return 1 ;;
    esac
    case "$CONTROLLER_URL" in
        https://|http://|*[!A-Za-z0-9:/._-]*) return 1 ;;
    esac
    if [ -n "$CONTROLLER_CA_FILE" ] && [ ! -r "$CONTROLLER_CA_FILE" ]; then
        return 1
    fi
    case "$CONTROLLER_URL" in
        http://*)
            case "$REQUIRE_TLS" in
                1|true|TRUE|yes|YES) return 1 ;;
            esac
            ;;
    esac
}

controller_curl() {
    if [ -n "$CONTROLLER_CA_FILE" ]; then
        set -- --cacert "$CONTROLLER_CA_FILE" "$@"
    fi
    if [ -n "$CONTROLLER_PINNED_PUBKEY" ]; then
        set -- --pinnedpubkey "$CONTROLLER_PINNED_PUBKEY" "$@"
    fi
    curl "$@"
}

transaction_valid_id() {
    case "$1" in
        ''|*[!A-Za-z0-9._-]*) return 1 ;;
    esac
    [ "$1" != "." ] && [ "$1" != ".." ] || return 1
    [ "${#1}" -le 128 ]
}

transaction_valid_plan_hash() {
    case "$1" in
        ''|*[!A-Fa-f0-9]*) return 1 ;;
    esac
    [ "${#1}" -eq 64 ]
}

transaction_valid_generation() {
    case "$1" in
        ''|*[!0-9]*) return 1 ;;
    esac
    [ "$1" -le 9223372036854775807 ] 2>/dev/null
}

transaction_valid_config() {
    case "$1" in
        wireless|network|dhcp|firewall|dropbear|system|sqm) return 0 ;;
    esac
    return 1
}

change_set_valid_config() {
    case "$1" in
        system|dhcp|firewall|dropbear|sqm) return 0 ;;
    esac
    return 1
}

transaction_identity_matches() {
    local transaction_path="$1"
    local transaction_config="$2"
    local expected_generation="${3:-}"
    local expected_plan_hash="${4:-}"
    local stored_config stored_generation stored_plan_hash

    stored_config=$(cat "$transaction_path/config" 2>/dev/null || true)
    if [ -n "$stored_config" ] && [ "$stored_config" != "$transaction_config" ]; then
        return 1
    fi

    if [ -f "$transaction_path/plan_hash" ]; then
        stored_plan_hash=$(cat "$transaction_path/plan_hash" 2>/dev/null || true)
    else
        stored_plan_hash=$(cat "$NERVE_TRANSACTION_ROOT/operation_${transaction_config}.hash" 2>/dev/null || true)
    fi
    if [ -n "$expected_plan_hash" ]; then
        if [ -n "$stored_plan_hash" ]; then
            [ "$stored_plan_hash" = "$expected_plan_hash" ] || return 1
        else
            return 1
        fi
    fi

    stored_generation=$(cat "$transaction_path/generation" 2>/dev/null || true)
    if [ -n "$expected_generation" ] && [ "$expected_generation" -gt 0 ] 2>/dev/null; then
        if [ -n "$stored_generation" ]; then
            [ "$stored_generation" = "$expected_generation" ] || return 1
        else
            return 1
        fi
    fi
    return 0
}

transaction_change_set_identity_matches() {
    local transaction_path="$1"
    local expected_change_set_id="$2"
    local expected_device_id="$3"
    local expected_generation="$4"
    local expected_plan_hash="$5"
    local expected_operation_id="$6"
    local expected_commands="$7"
    local expected_observed_state_hash="$8"
    local expected_health_checks="$9"
    local expected_policy="${10}"
    local stored_change_set_id stored_device_id stored_operation_id stored_commands
    local stored_observed_state_hash stored_health_checks stored_policy

    transaction_identity_matches "$transaction_path" system "$expected_generation" "$expected_plan_hash" || return 1
    stored_change_set_id=$(cat "$transaction_path/change_set_id" 2>/dev/null || true)
    stored_device_id=$(cat "$transaction_path/change_set_device_id" 2>/dev/null || true)
    stored_operation_id=$(cat "$transaction_path/change_set_operation_id" 2>/dev/null || true)
    stored_commands=$(cat "$transaction_path/change_set_commands" 2>/dev/null || true)
    stored_observed_state_hash=$(cat "$transaction_path/change_set_observed_state_hash" 2>/dev/null || true)
    stored_health_checks=$(cat "$transaction_path/change_set_health_checks" 2>/dev/null || true)
    stored_policy=$(cat "$transaction_path/change_set_confirmation_policy" 2>/dev/null || true)
    [ "$stored_change_set_id" = "$expected_change_set_id" ] || return 1
    [ "$(printf '%s' "$stored_device_id" | tr '[:lower:]' '[:upper:]')" = "$(printf '%s' "$expected_device_id" | tr '[:lower:]' '[:upper:]')" ] || return 1
    [ "$stored_operation_id" = "$expected_operation_id" ] || return 1
    [ "$stored_commands" = "$expected_commands" ] || return 1
    [ "$stored_observed_state_hash" = "$expected_observed_state_hash" ] || return 1
    [ "$stored_health_checks" = "$expected_health_checks" ] || return 1
    [ "$stored_policy" = "$expected_policy" ] || return 1
    return 0
}

transaction_write_atomic() {
    local transaction_file="$1"
    local transaction_value="$2"
    local transaction_tmp="$transaction_file.$$"
    printf '%s\n' "$transaction_value" > "$transaction_tmp" || return 1
    chmod 600 "$transaction_tmp" 2>/dev/null || return 1
    mv "$transaction_tmp" "$transaction_file" || return 1
    if command -v sync >/dev/null 2>&1; then
        sync
    fi
    return 0
}

transaction_mark_recovery_required() {
    local transaction_path="$1"
    local recovery_reason="${2:-transaction recovery failed}"
    transaction_write_atomic "$transaction_path/failure" "$recovery_reason" || return 1
    transaction_write_atomic "$transaction_path/state" RECOVERY_REQUIRED || return 1
}

transaction_mark_change_set_recovery_required() {
    local transaction_path="$1"
    local recovery_reason="${2:-changeset journal is not recoverable}"
    local transaction_id change_set_device_id change_set_plan_hash change_set_generation
    transaction_id=${transaction_path##*/}
    change_set_device_id=$(cat "$transaction_path/change_set_device_id" 2>/dev/null || true)
    change_set_plan_hash=$(cat "$transaction_path/change_set_plan_hash" 2>/dev/null || true)
    change_set_generation=$(cat "$transaction_path/change_set_generation" 2>/dev/null || true)
    if transaction_valid_id "$transaction_id" && change_set_valid_device_id "$change_set_device_id" &&
        transaction_valid_plan_hash "$change_set_plan_hash" && transaction_valid_generation "$change_set_generation" &&
        [ "$change_set_generation" -gt 0 ] 2>/dev/null; then
        change_set_status_write "$transaction_id" "$change_set_device_id" "$change_set_plan_hash" "$change_set_generation" RECOVERY_REQUIRED "$recovery_reason" || true
    fi
    transaction_mark_recovery_required "$transaction_path" "$recovery_reason"
}

agent_secret_write() {
    local secret_file="$1"
    local secret_value="$2"
    local secret_dir="${secret_file%/*}"
    local secret_tmp="$secret_file.$$"
    [ "$secret_dir" = "$secret_file" ] && secret_dir="."
    mkdir -p "$secret_dir" || return 1
    printf '%s\n' "$secret_value" > "$secret_tmp" || return 1
    chmod 600 "$secret_tmp" 2>/dev/null || return 1
    mv "$secret_tmp" "$secret_file" || return 1
    command -v sync >/dev/null 2>&1 && sync
    return 0
}

# Verify an Ed25519 signature without ever placing decoded binary data in a
# shell variable. The public key is raw 32-byte base64; OpenSSL expects the
# SubjectPublicKeyInfo DER wrapper built below.
verify_agent_artifact() {
    local artifact_file="$1"
    local signature_base64="$2"
    local public_key_base64="$3"
    local verify_dir="/tmp/nerve-agent-verify.$$"
    local public_key_size
    local signature_size
    mkdir "$verify_dir" 2>/dev/null || return 1
    if ! printf '%s' "$public_key_base64" | decode_base64 > "$verify_dir/public.raw" 2>/dev/null; then
        rm -rf "$verify_dir"
        return 1
    fi
    public_key_size=$(wc -c < "$verify_dir/public.raw")
    if [ "$public_key_size" -ne 32 ]; then
        rm -rf "$verify_dir"
        return 1
    fi
    printf '\060\052\060\005\006\003\053\145\160\003\041\000' > "$verify_dir/public.der" || {
        rm -rf "$verify_dir"
        return 1
    }
    cat "$verify_dir/public.raw" >> "$verify_dir/public.der" || {
        rm -rf "$verify_dir"
        return 1
    }
    if ! printf '%s' "$signature_base64" | decode_base64 > "$verify_dir/signature" 2>/dev/null; then
        rm -rf "$verify_dir"
        return 1
    fi
    signature_size=$(wc -c < "$verify_dir/signature")
    if [ "$signature_size" -ne 64 ] || ! command -v openssl >/dev/null 2>&1; then
        rm -rf "$verify_dir"
        return 1
    fi
    openssl pkeyutl -verify -pubin -inkey "$verify_dir/public.der" -rawin \
        -in "$artifact_file" -sigfile "$verify_dir/signature" >/dev/null 2>&1
    local verify_status=$?
    rm -rf "$verify_dir"
    return "$verify_status"
}

transaction_dir() {
    printf '%s/%s\n' "$NERVE_TRANSACTION_ROOT" "$1"
}

transaction_restart_config() {
    case "$1" in
        wireless)
            if command -v wifi >/dev/null 2>&1; then wifi reload; fi
            ;;
        network)
            if [ -x /etc/init.d/network ]; then /etc/init.d/network reload; fi
            ;;
        dhcp)
            if [ -x /etc/init.d/dnsmasq ]; then /etc/init.d/dnsmasq reload; fi
            ;;
        firewall)
            if [ -x /etc/init.d/firewall ]; then /etc/init.d/firewall reload; fi
            ;;
        dropbear)
            if [ -x /etc/init.d/dropbear ]; then /etc/init.d/dropbear reload; fi
            ;;
        sqm)
            if [ -x /etc/init.d/sqm ]; then /etc/init.d/sqm reload; fi
            ;;
        system)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

change_set_valid_device_id() {
    case "$1" in
        ''|*[!A-Za-z0-9:._-]*) return 1 ;;
    esac
    [ "${#1}" -le 50 ]
}

change_set_valid_state() {
    case "$1" in
        PREPARED|APPLYING|PENDING_CONFIRM|ROLLING_BACK|COMMITTED|RESTORED|RECOVERY_REQUIRED|REJECTED) return 0 ;;
    esac
    return 1
}

change_set_transition_allowed() {
    local previous_state="$1"
    local next_state="$2"
    case "$previous_state:$next_state" in
        :PREPARED|:REJECTED|:ROLLING_BACK|:RECOVERY_REQUIRED)
            return 0
            ;;
        PREPARED:PREPARED|PREPARED:APPLYING|PREPARED:REJECTED|PREPARED:ROLLING_BACK|PREPARED:RECOVERY_REQUIRED)
            return 0
            ;;
        APPLYING:APPLYING|APPLYING:COMMITTED|APPLYING:PENDING_CONFIRM|APPLYING:ROLLING_BACK|APPLYING:RESTORED|APPLYING:RECOVERY_REQUIRED|APPLYING:REJECTED)
            return 0
            ;;
        PENDING_CONFIRM:PENDING_CONFIRM|PENDING_CONFIRM:COMMITTED|PENDING_CONFIRM:ROLLING_BACK|PENDING_CONFIRM:RESTORED|PENDING_CONFIRM:RECOVERY_REQUIRED)
            return 0
            ;;
        ROLLING_BACK:ROLLING_BACK|ROLLING_BACK:RESTORED|ROLLING_BACK:RECOVERY_REQUIRED)
            return 0
            ;;
        COMMITTED:COMMITTED|RESTORED:RESTORED|RESTORED:PREPARED|RESTORED:REJECTED|RECOVERY_REQUIRED:RECOVERY_REQUIRED|REJECTED:REJECTED)
            return 0
            ;;
    esac
    return 1
}

change_set_status_write() {
    local change_set_id="$1"
    local change_set_device_id="$2"
    local change_set_plan_hash="$3"
    local change_set_generation="$4"
    local change_set_state="$5"
    local change_set_failure="${6:-}"
    local escaped_failure existing_status existing_id existing_state existing_device_id existing_plan_hash existing_generation

    transaction_valid_id "$change_set_id" || return 1
    change_set_valid_device_id "$change_set_device_id" || return 1
    transaction_valid_plan_hash "$change_set_plan_hash" || return 1
    transaction_valid_generation "$change_set_generation" || return 1
    [ "$change_set_generation" -gt 0 ] 2>/dev/null || return 1
    change_set_valid_state "$change_set_state" || return 1
    mkdir -p "$NERVE_TRANSACTION_ROOT" || return 1
    chmod 700 "$NERVE_TRANSACTION_ROOT" 2>/dev/null || return 1

    if [ -s "$NERVE_CHANGE_SET_STATUS_FILE" ]; then
        existing_status=$(cat "$NERVE_CHANGE_SET_STATUS_FILE" 2>/dev/null || true)
        existing_id=$(printf '%s' "$existing_status" | jsonfilter -e '@.change_set_id' 2>/dev/null || true)
        if [ "$existing_id" = "$change_set_id" ]; then
            existing_device_id=$(printf '%s' "$existing_status" | jsonfilter -e '@.device_id' 2>/dev/null || true)
            existing_plan_hash=$(printf '%s' "$existing_status" | jsonfilter -e '@.plan_hash' 2>/dev/null || true)
            existing_generation=$(printf '%s' "$existing_status" | jsonfilter -e '@.generation' 2>/dev/null || true)
            if [ "$(printf '%s' "$existing_device_id" | tr '[:lower:]' '[:upper:]')" != "$(printf '%s' "$change_set_device_id" | tr '[:lower:]' '[:upper:]')" ]; then
                return 1
            fi
            [ "$existing_plan_hash" = "$change_set_plan_hash" ] || return 1
            [ "$existing_generation" = "$change_set_generation" ] || return 1
            existing_state=$(printf '%s' "$existing_status" | jsonfilter -e '@.state' 2>/dev/null || true)
            change_set_transition_allowed "$existing_state" "$change_set_state" || return 1
        elif [ -n "$existing_id" ]; then
            existing_state=$(printf '%s' "$existing_status" | jsonfilter -e '@.state' 2>/dev/null || true)
            case "$existing_state" in
                PREPARED|APPLYING|PENDING_CONFIRM|ROLLING_BACK|RECOVERY_REQUIRED)
                    return 1
                    ;;
            esac
            existing_generation=$(printf '%s' "$existing_status" | jsonfilter -e '@.generation' 2>/dev/null || true)
            transaction_valid_generation "$existing_generation" || return 1
            [ "$change_set_generation" -gt "$existing_generation" ] 2>/dev/null || return 1
        fi
    fi

    if [ -n "$change_set_failure" ]; then
        escaped_failure=$(printf '%s' "$change_set_failure" | tr '\r\n' '  ' | sed 's/\\/\\\\/g; s/"/\\"/g')
        transaction_write_atomic "$NERVE_CHANGE_SET_STATUS_FILE" "{\"change_set_id\":\"$change_set_id\",\"device_id\":\"$change_set_device_id\",\"plan_hash\":\"$change_set_plan_hash\",\"generation\":$change_set_generation,\"state\":\"$change_set_state\",\"failure\":\"$escaped_failure\"}"
    else
        transaction_write_atomic "$NERVE_CHANGE_SET_STATUS_FILE" "{\"change_set_id\":\"$change_set_id\",\"device_id\":\"$change_set_device_id\",\"plan_hash\":\"$change_set_plan_hash\",\"generation\":$change_set_generation,\"state\":\"$change_set_state\"}"
    fi
}

change_set_status_json() {
    if [ -s "$NERVE_CHANGE_SET_STATUS_FILE" ]; then
        cat "$NERVE_CHANGE_SET_STATUS_FILE"
    else
        printf '{}'
    fi
}

transaction_recover_orphan_change_set_status() {
    local status_json status_id status_device_id status_plan_hash status_generation status_state transaction_path
    [ -s "$NERVE_CHANGE_SET_STATUS_FILE" ] || return 0
    status_json=$(change_set_status_json)
    status_id=$(printf '%s' "$status_json" | jsonfilter -e '@.change_set_id' 2>/dev/null || true)
    status_device_id=$(printf '%s' "$status_json" | jsonfilter -e '@.device_id' 2>/dev/null || true)
    status_plan_hash=$(printf '%s' "$status_json" | jsonfilter -e '@.plan_hash' 2>/dev/null || true)
    status_generation=$(printf '%s' "$status_json" | jsonfilter -e '@.generation' 2>/dev/null || true)
    status_state=$(printf '%s' "$status_json" | jsonfilter -e '@.state' 2>/dev/null || true)
    transaction_valid_id "$status_id" || return 1
    change_set_valid_device_id "$status_device_id" || return 1
    transaction_valid_plan_hash "$status_plan_hash" || return 1
    transaction_valid_generation "$status_generation" || return 1
    change_set_valid_state "$status_state" || return 1
    case "$status_state" in
        PREPARED|APPLYING|PENDING_CONFIRM|ROLLING_BACK)
            transaction_path=$(transaction_dir "$status_id")
            if [ ! -f "$transaction_path/state" ]; then
                change_set_status_write "$status_id" "$status_device_id" "$status_plan_hash" "$status_generation" RECOVERY_REQUIRED "changeset journal is missing" || return 1
                return 1
            fi
            ;;
    esac
    return 0
}

change_set_generation_allowed() {
    local change_set_id="$1"
    local change_set_generation="$2"
    local status_json status_id status_generation status_state transaction_path transaction_id transaction_state transaction_generation
    local highest_generation=0 highest_change_set_id=""

    status_json=$(change_set_status_json)
    status_id=$(printf '%s' "$status_json" | jsonfilter -e '@.change_set_id' 2>/dev/null || true)
    status_generation=$(printf '%s' "$status_json" | jsonfilter -e '@.generation' 2>/dev/null || true)
    status_state=$(printf '%s' "$status_json" | jsonfilter -e '@.state' 2>/dev/null || true)
    if [ "$status_state" = "RECOVERY_REQUIRED" ]; then
        return 1
    fi
    if transaction_valid_generation "$status_generation" && [ "$status_generation" -gt 0 ] 2>/dev/null; then
        highest_generation="$status_generation"
        highest_change_set_id="$status_id"
    fi

    for transaction_path in "$NERVE_TRANSACTION_ROOT"/*; do
        [ -d "$transaction_path" ] || continue
        transaction_id=${transaction_path##*/}
        transaction_state=$(cat "$transaction_path/state" 2>/dev/null || true)
        [ "$transaction_state" = "RECOVERY_REQUIRED" ] && return 1
        transaction_generation=$(cat "$transaction_path/change_set_generation" 2>/dev/null || true)
        [ -n "$transaction_generation" ] || transaction_generation=$(cat "$transaction_path/generation" 2>/dev/null || true)
        transaction_valid_generation "$transaction_generation" || continue
        [ "$transaction_generation" -gt 0 ] 2>/dev/null || continue
        if [ "$transaction_generation" -gt "$highest_generation" ] 2>/dev/null; then
            highest_generation="$transaction_generation"
            if [ -f "$transaction_path/change_set_device_id" ]; then
                highest_change_set_id="$transaction_id"
            else
                highest_change_set_id=""
            fi
        fi
    done

    if [ "$change_set_generation" -gt "$highest_generation" ] 2>/dev/null; then
        return 0
    fi
    [ "$change_set_generation" -eq "$highest_generation" ] 2>/dev/null && [ "$highest_change_set_id" = "$change_set_id" ]
}

transaction_publish_change_set_status() {
    local transaction_id="$1"
    local transaction_path transaction_state change_set_device_id change_set_plan_hash change_set_generation change_set_failure
    transaction_path=$(transaction_dir "$transaction_id")
    transaction_state=$(cat "$transaction_path/state" 2>/dev/null || true)
    case "$transaction_state" in
        COMMITTED|RESTORED|RECOVERY_REQUIRED) ;;
        *) return 1 ;;
    esac
    change_set_device_id=$(cat "$transaction_path/change_set_device_id" 2>/dev/null || true)
    change_set_plan_hash=$(cat "$transaction_path/change_set_plan_hash" 2>/dev/null || true)
    change_set_generation=$(cat "$transaction_path/change_set_generation" 2>/dev/null || true)
    change_set_failure=$(cat "$transaction_path/failure" 2>/dev/null || true)
    [ -n "$change_set_device_id" ] && [ -n "$change_set_plan_hash" ] && [ -n "$change_set_generation" ] || return 1
    change_set_status_write "$transaction_id" "$change_set_device_id" "$change_set_plan_hash" "$change_set_generation" "$transaction_state" "$change_set_failure"
}

change_set_transaction_begin() {
    local transaction_id="$1"
    local namespaces="$2"
    local transaction_path namespace transaction_tmp
    transaction_valid_id "$transaction_id" || return 1
    mkdir -p "$NERVE_TRANSACTION_ROOT" || return 1
    transaction_path=$(transaction_dir "$transaction_id")
    mkdir -p "$transaction_path" || return 1
    chmod 700 "$transaction_path" 2>/dev/null || return 1
    transaction_write_atomic "$transaction_path/change_set_namespaces" "$namespaces" || return 1
    transaction_write_atomic "$transaction_path/config" changeset || return 1
    while IFS= read -r namespace; do
        [ -n "$namespace" ] || continue
        change_set_valid_config "$namespace" || return 1
        if [ -f "$NERVE_CONFIG_ROOT/$namespace" ]; then
            transaction_tmp="$transaction_path/backup_${namespace}.$$"
            cp "$NERVE_CONFIG_ROOT/$namespace" "$transaction_tmp" || return 1
            chmod 600 "$transaction_tmp" 2>/dev/null || return 1
            mv "$transaction_tmp" "$transaction_path/backup_$namespace" || return 1
            transaction_write_atomic "$transaction_path/backup_exists_$namespace" 1 || return 1
        else
            transaction_write_atomic "$transaction_path/backup_exists_$namespace" 0 || return 1
        fi
    done <<EOF
$namespaces
EOF
    transaction_write_atomic "$transaction_path/state" APPLYING || return 1
    transaction_write_atomic "$NERVE_TRANSACTION_ROOT/active" "$transaction_id" || return 1
}

change_set_recover_snapshots() {
    local transaction_path="$1"
    local transaction_id="${transaction_path##*/}"
    local namespaces namespace transaction_tmp
    namespaces=$(cat "$transaction_path/change_set_namespaces" 2>/dev/null || true)
    [ -n "$namespaces" ] || return 1
    transaction_write_atomic "$transaction_path/state" ROLLING_BACK || return 1
    while IFS= read -r namespace; do
        [ -n "$namespace" ] || continue
        change_set_valid_config "$namespace" || return 1
        if [ "$(cat "$transaction_path/backup_exists_$namespace" 2>/dev/null || true)" = 1 ]; then
            transaction_tmp="$NERVE_CONFIG_ROOT/$namespace.$$"
            cp "$transaction_path/backup_$namespace" "$transaction_tmp" || return 1
            mv "$transaction_tmp" "$NERVE_CONFIG_ROOT/$namespace" || return 1
        else
            rm -f "$NERVE_CONFIG_ROOT/$namespace" || return 1
        fi
        if command -v uci >/dev/null 2>&1; then
            uci revert "$namespace" 2>/dev/null || return 1
            uci commit "$namespace" || return 1
        fi
        transaction_restart_config "$namespace" || return 1
    done <<EOF
$namespaces
EOF
    transaction_write_atomic "$transaction_path/state" RESTORED || return 1
    if [ "$(cat "$NERVE_TRANSACTION_ROOT/active" 2>/dev/null || true)" = "$transaction_id" ]; then
        rm -f "$NERVE_TRANSACTION_ROOT/active" || return 1
    fi
    rm -f "$transaction_path"/backup_* "$transaction_path/change_set_namespaces" || return 1
    transaction_write_atomic "$NERVE_TRANSACTION_ROOT/last" "$transaction_id" || return 1
}

change_set_reject() {
    local rejection_reason="$1"
    change_set_status_write "$change_set_id" "$change_set_device_id" "$change_set_plan_hash" "$change_set_generation" REJECTED "$rejection_reason" || true
    return 1
}

change_set_content_plan_hash() {
    local change_set_commands="$1"
    local change_set_health_checks="$2"
    local change_set_observed_hash="$3"
    printf '{"operations":[{"config":"system","commands":%s,"observed_state_hash":"%s"}],"health_checks":%s,"confirmation_policy":"local_auto"}' \
        "$change_set_commands" "$change_set_observed_hash" "$change_set_health_checks" | sha256sum | awk '{print $1}'
}

change_set_content_plan_hash_multi() {
    local change_set_operations="$1"
    local change_set_health_checks="$2"
    local change_set_policy="$3"
    printf '{"operations":%s,"health_checks":%s,"confirmation_policy":"%s"}' \
        "$change_set_operations" "$change_set_health_checks" "$change_set_policy" | sha256sum | awk '{print $1}'
}

change_set_transaction_identity_matches_multi() {
    local transaction_path="$1"
    local expected_id="$2" expected_device="$3" expected_hash="$4" expected_generation="$5"
    local expected_operations="$6" expected_health_checks="$7" expected_policy="$8"
    [ "$(cat "$transaction_path/change_set_plan_hash" 2>/dev/null || true)" = "$expected_hash" ] || return 1
    [ "$(cat "$transaction_path/change_set_generation" 2>/dev/null || true)" = "$expected_generation" ] || return 1
    [ "$(cat "$transaction_path/change_set_id" 2>/dev/null || true)" = "$expected_id" ] || return 1
    [ "$(cat "$transaction_path/change_set_device_id" 2>/dev/null || true)" = "$expected_device" ] || return 1
    [ "$(cat "$transaction_path/change_set_operations" 2>/dev/null || true)" = "$expected_operations" ] || return 1
    [ "$(cat "$transaction_path/change_set_health_checks" 2>/dev/null || true)" = "$expected_health_checks" ] || return 1
    [ "$(cat "$transaction_path/change_set_confirmation_policy" 2>/dev/null || true)" = "$expected_policy" ] || return 1
}

transaction_recover_one() {
    local transaction_id="$1"
    local transaction_path transaction_state transaction_config transaction_tmp manifest_file
    local change_set_device_id change_set_plan_hash change_set_generation transaction_is_change_set
    if ! transaction_valid_id "$transaction_id"; then
        logger -t agent "TRANSACTION_RECOVERY_FAILED: invalid active operation id"
        return 1
    fi

    transaction_path=$(transaction_dir "$transaction_id")
    transaction_is_change_set=0
    [ -f "$transaction_path/change_set_device_id" ] || [ -f "$transaction_path/change_set_namespaces" ] && transaction_is_change_set=1
    if [ "$transaction_is_change_set" -eq 1 ]; then
        for manifest_file in change_set_id change_set_plan_hash change_set_generation change_set_operation_id change_set_commands change_set_observed_state_hash change_set_health_checks change_set_confirmation_policy; do
            if [ ! -s "$transaction_path/$manifest_file" ]; then
            transaction_mark_change_set_recovery_required "$transaction_path" "changeset manifest is incomplete" || true
                return 1
            fi
        done
    fi
    transaction_state=$(cat "$transaction_path/state" 2>/dev/null || true)
    case "$transaction_state" in
        COMMITTED)
            if [ "$transaction_is_change_set" -eq 1 ]; then
                transaction_publish_change_set_status "$transaction_id" || return 1
            fi
            if [ "$(cat "$NERVE_TRANSACTION_ROOT/active" 2>/dev/null || true)" = "$transaction_id" ]; then
                rm -f "$NERVE_TRANSACTION_ROOT/active"
            fi
            return 0
            ;;
        RESTORED)
            if [ "$transaction_is_change_set" -eq 1 ]; then
                transaction_publish_change_set_status "$transaction_id" || return 1
            fi
            if [ "$(cat "$NERVE_TRANSACTION_ROOT/active" 2>/dev/null || true)" = "$transaction_id" ]; then
                rm -f "$NERVE_TRANSACTION_ROOT/active"
            fi
            return 0
            ;;
        RECOVERY_REQUIRED)
            if [ "$transaction_is_change_set" -eq 1 ]; then
                transaction_publish_change_set_status "$transaction_id" || return 1
            fi
            logger -t agent "TRANSACTION_RECOVERY_REQUIRED: refusing to continue $transaction_id"
            return 1
            ;;
        APPLYING|PENDING_CONFIRM|ROLLING_BACK)
            ;;
        *)
            logger -t agent "TRANSACTION_RECOVERY_FAILED: unknown state $transaction_state"
            if [ "$transaction_is_change_set" -eq 1 ]; then
                transaction_mark_change_set_recovery_required "$transaction_path" "unknown changeset journal state" || true
            fi
            return 1
            ;;
    esac

    if [ "$transaction_is_change_set" -eq 1 ] && [ -f "$transaction_path/change_set_namespaces" ]; then
        if change_set_recover_snapshots "$transaction_path"; then
            if [ "$transaction_is_change_set" -eq 1 ]; then
                transaction_publish_change_set_status "$transaction_id" || return 1
            fi
            return 0
        fi
        transaction_mark_change_set_recovery_required "$transaction_path" "transaction recovery failed" || true
        return 1
    fi

    transaction_config=$(cat "$transaction_path/config" 2>/dev/null || true)
    if ! transaction_valid_config "$transaction_config"; then
        logger -t agent "TRANSACTION_RECOVERY_FAILED: invalid config namespace"
        transaction_mark_recovery_required "$transaction_path"
        return 1
    fi

    if [ "$transaction_is_change_set" -eq 1 ]; then
        change_set_device_id=$(cat "$transaction_path/change_set_device_id" 2>/dev/null || true)
        change_set_plan_hash=$(cat "$transaction_path/change_set_plan_hash" 2>/dev/null || true)
        change_set_generation=$(cat "$transaction_path/change_set_generation" 2>/dev/null || true)
        change_set_status_write "$transaction_id" "$change_set_device_id" "$change_set_plan_hash" "$change_set_generation" ROLLING_BACK "changeset rollback started" || {
            transaction_mark_recovery_required "$transaction_path" "changeset rollback status could not be persisted"
            return 1
        }
    fi
    transaction_write_atomic "$transaction_path/state" ROLLING_BACK || {
        transaction_mark_recovery_required "$transaction_path"
        return 1
    }
    if [ "$(cat "$transaction_path/backup_exists" 2>/dev/null || true)" = "1" ]; then
        transaction_tmp="$NERVE_CONFIG_ROOT/$transaction_config.$$"
        cp "$transaction_path/backup" "$transaction_tmp" || {
            rm -f "$transaction_tmp"
            transaction_mark_recovery_required "$transaction_path"
            return 1
        }
        mv "$transaction_tmp" "$NERVE_CONFIG_ROOT/$transaction_config" || {
            rm -f "$transaction_tmp"
            transaction_mark_recovery_required "$transaction_path"
            return 1
        }
    else
        rm -f "$NERVE_CONFIG_ROOT/$transaction_config" || {
            transaction_mark_recovery_required "$transaction_path"
            return 1
        }
    fi
    if command -v uci >/dev/null 2>&1; then
        uci revert "$transaction_config" 2>/dev/null || {
            transaction_mark_recovery_required "$transaction_path"
            return 1
        }
        uci commit "$transaction_config" || {
            transaction_mark_recovery_required "$transaction_path"
            return 1
        }
    fi
    transaction_restart_config "$transaction_config" || {
        transaction_mark_recovery_required "$transaction_path"
        return 1
    }
    transaction_write_atomic "$transaction_path/state" RESTORED || {
        transaction_mark_recovery_required "$transaction_path"
        return 1
    }
    if [ "$(cat "$NERVE_TRANSACTION_ROOT/active" 2>/dev/null || true)" = "$transaction_id" ]; then
        rm -f "$NERVE_TRANSACTION_ROOT/active" || {
            transaction_mark_recovery_required "$transaction_path"
            return 1
        }
    fi
    rm -f "$transaction_path/backup" "$transaction_path/backup_exists" || {
        transaction_mark_recovery_required "$transaction_path"
        return 1
    }
    transaction_write_atomic "$NERVE_TRANSACTION_ROOT/last" "$transaction_id" || {
        transaction_mark_recovery_required "$transaction_path"
        return 1
    }
    if [ "$transaction_is_change_set" -ne 1 ]; then
        transaction_write_atomic "$NERVE_OPERATION_STATUS_FILE" "$transaction_id" || return 1
    fi
    change_set_device_id=$(cat "$transaction_path/change_set_device_id" 2>/dev/null || true)
    change_set_plan_hash=$(cat "$transaction_path/change_set_plan_hash" 2>/dev/null || true)
    change_set_generation=$(cat "$transaction_path/change_set_generation" 2>/dev/null || true)
    if [ "$transaction_is_change_set" -eq 1 ]; then
        transaction_publish_change_set_status "$transaction_id" || return 1
    fi
    logger -t agent "TRANSACTION_RECOVERED: restored $transaction_config for $transaction_id"
}

transaction_recover_terminal_change_set() {
    local transaction_path transaction_id transaction_state transaction_generation
    local highest_path="" highest_id="" highest_generation=0 current_status current_id current_generation
    for transaction_path in "$NERVE_TRANSACTION_ROOT"/*; do
        [ -d "$transaction_path" ] || continue
        [ -f "$transaction_path/change_set_device_id" ] || continue
        transaction_state=$(cat "$transaction_path/state" 2>/dev/null || true)
        case "$transaction_state" in
            COMMITTED|RESTORED) ;;
            *) continue ;;
        esac
        transaction_generation=$(cat "$transaction_path/change_set_generation" 2>/dev/null || true)
        transaction_valid_generation "$transaction_generation" || return 1
        [ "$transaction_generation" -gt 0 ] 2>/dev/null || return 1
        transaction_id=${transaction_path##*/}
        if [ "$transaction_generation" -gt "$highest_generation" ] 2>/dev/null; then
            highest_path="$transaction_path"
            highest_id="$transaction_id"
            highest_generation="$transaction_generation"
        elif [ "$transaction_generation" -eq "$highest_generation" ] 2>/dev/null && [ "$highest_generation" -gt 0 ]; then
            logger -t agent "TRANSACTION_RECOVERY_FAILED: duplicate changeset generation $transaction_generation"
            return 1
        fi
    done
    [ -n "$highest_path" ] || return 0

    current_status=$(change_set_status_json)
    current_id=$(printf '%s' "$current_status" | jsonfilter -e '@.change_set_id' 2>/dev/null || true)
    current_generation=$(printf '%s' "$current_status" | jsonfilter -e '@.generation' 2>/dev/null || true)
    if [ -n "$current_id" ] && [ "$current_id" != "$highest_id" ] &&
        transaction_valid_generation "$current_generation" &&
        [ "$current_generation" -ge "$highest_generation" ] 2>/dev/null; then
        return 0
    fi
    transaction_recover_one "$highest_id"
}

transaction_recover_pending() {
    local transaction_id transaction_path transaction_state
    transaction_recover_orphan_change_set_status || return 1
    if [ -f "$NERVE_TRANSACTION_ROOT/active" ]; then
        transaction_id=$(cat "$NERVE_TRANSACTION_ROOT/active" 2>/dev/null || true)
        transaction_recover_one "$transaction_id" || return 1
    fi

    # A crash between the journal state write and the active marker must not
    # leave an APPLYING transaction permanently blocking future changes.
    for transaction_path in "$NERVE_TRANSACTION_ROOT"/*; do
        [ -d "$transaction_path" ] || continue
        transaction_id=${transaction_path##*/}
        transaction_state=$(cat "$transaction_path/state" 2>/dev/null || true)
        case "$transaction_state" in
            APPLYING|PENDING_CONFIRM|ROLLING_BACK)
                if [ "$(cat "$NERVE_TRANSACTION_ROOT/active" 2>/dev/null || true)" != "$transaction_id" ]; then
                    transaction_recover_one "$transaction_id" || return 1
                fi
                ;;
            RECOVERY_REQUIRED)
                transaction_recover_one "$transaction_id" || return 1
                ;;
            COMMITTED|RESTORED)
                ;;
            PREPARED)
                transaction_recover_one "$transaction_id" || return 1
                ;;
            '')
                transaction_recover_one "$transaction_id" || return 1
                ;;
            *)
                transaction_recover_one "$transaction_id" || return 1
                ;;
        esac
    done
    transaction_recover_terminal_change_set || return 1
}

transaction_prune_terminal() {
    local transaction_path transaction_id transaction_state last_id operation_id active_id
    last_id=$(cat "$NERVE_TRANSACTION_ROOT/last" 2>/dev/null || true)
    operation_id=$(cat "$NERVE_OPERATION_STATUS_FILE" 2>/dev/null || true)
    active_id=$(cat "$NERVE_TRANSACTION_ROOT/active" 2>/dev/null || true)
    for transaction_path in "$NERVE_TRANSACTION_ROOT"/*; do
        [ -d "$transaction_path" ] || continue
        transaction_id=${transaction_path##*/}
        [ "$transaction_id" = "$last_id" ] && continue
        [ "$transaction_id" = "$operation_id" ] && continue
        [ "$transaction_id" = "$active_id" ] && continue
        transaction_state=$(cat "$transaction_path/state" 2>/dev/null || true)
        case "$transaction_state" in
            COMMITTED|RESTORED) rm -rf "$transaction_path" ;;
        esac
    done
}

transaction_begin() {
    local transaction_config="$1"
    local transaction_id="$2"
    local transaction_generation="${3:-}"
    local transaction_plan_hash="${4:-}"
    local active_id active_state transaction_path transaction_state transaction_tmp
    transaction_valid_config "$transaction_config" || return 1
    transaction_valid_id "$transaction_id" || return 1
    if [ -n "$transaction_generation" ]; then
        transaction_valid_generation "$transaction_generation" || return 1
    fi
    if [ -n "$transaction_plan_hash" ]; then
        transaction_valid_plan_hash "$transaction_plan_hash" || return 1
    fi
    mkdir -p "$NERVE_TRANSACTION_ROOT" || return 1
    chmod 700 "$NERVE_TRANSACTION_ROOT" 2>/dev/null || return 1

    # Recovery-required journals fence every later mutation, even if an
    # interrupted process lost the active marker.
    transaction_recover_pending || return 1

    if [ -f "$NERVE_TRANSACTION_ROOT/active" ]; then
        active_id=$(cat "$NERVE_TRANSACTION_ROOT/active" 2>/dev/null || true)
        if [ "$active_id" != "$transaction_id" ]; then
            transaction_recover_pending || return 1
        else
            active_state=$(cat "$(transaction_dir "$active_id")/state" 2>/dev/null || true)
            if [ "$active_state" != "COMMITTED" ]; then
                transaction_recover_pending || return 1
            fi
        fi
        if [ -f "$NERVE_TRANSACTION_ROOT/active" ]; then
            active_state=$(cat "$(transaction_dir "$active_id")/state" 2>/dev/null || true)
            if [ "$active_id" = "$transaction_id" ] && [ "$active_state" = "COMMITTED" ]; then
                transaction_identity_matches "$(transaction_dir "$transaction_id")" "$transaction_config" "$transaction_generation" "$transaction_plan_hash" || return 1
                return 10
            fi
            return 1
        fi
    fi

    transaction_path=$(transaction_dir "$transaction_id")
    if [ -f "$transaction_path/state" ]; then
        transaction_state=$(cat "$transaction_path/state" 2>/dev/null || true)
        case "$transaction_state" in
            COMMITTED)
                transaction_identity_matches "$transaction_path" "$transaction_config" "$transaction_generation" "$transaction_plan_hash" || return 1
                return 10
                ;;
            APPLYING|PENDING_CONFIRM|ROLLING_BACK)
                return 1
                ;;
            RESTORED)
                transaction_identity_matches "$transaction_path" "$transaction_config" "$transaction_generation" "$transaction_plan_hash" || return 1
                rm -rf "$transaction_path"
                ;;
            RECOVERY_REQUIRED)
                return 1
                ;;
            *)
                return 1
                ;;
        esac
    fi

    mkdir -p "$transaction_path" || return 1
    chmod 700 "$transaction_path" 2>/dev/null || return 1
    if [ -f "$NERVE_CONFIG_ROOT/$transaction_config" ]; then
        transaction_tmp="$transaction_path/backup.$$"
        cp "$NERVE_CONFIG_ROOT/$transaction_config" "$transaction_tmp" || return 1
        chmod 600 "$transaction_tmp" 2>/dev/null || return 1
        mv "$transaction_tmp" "$transaction_path/backup" || return 1
        command -v sync >/dev/null 2>&1 && sync
        transaction_write_atomic "$transaction_path/backup_exists" 1 || return 1
    else
        transaction_write_atomic "$transaction_path/backup_exists" 0 || return 1
    fi
    transaction_write_atomic "$transaction_path/config" "$transaction_config" || return 1
    if [ -n "$transaction_plan_hash" ]; then
        transaction_write_atomic "$transaction_path/plan_hash" "$transaction_plan_hash" || return 1
    else
        rm -f "$transaction_path/plan_hash"
    fi
    if [ -n "$transaction_generation" ] && [ "$transaction_generation" -gt 0 ] 2>/dev/null; then
        transaction_write_atomic "$transaction_path/generation" "$transaction_generation" || return 1
    else
        rm -f "$transaction_path/generation"
    fi
    if [ "${TRANSACTION_CHANGE_SET:-0}" = "1" ]; then
        transaction_valid_id "$TRANSACTION_CHANGE_SET_ID" || return 1
        [ -n "$TRANSACTION_CHANGE_SET_DEVICE_ID" ] || return 1
        transaction_valid_plan_hash "$TRANSACTION_CHANGE_SET_PLAN_HASH" || return 1
        transaction_valid_generation "$TRANSACTION_CHANGE_SET_GENERATION" || return 1
        [ -n "$TRANSACTION_CHANGE_SET_COMMANDS" ] || return 1
        transaction_valid_plan_hash "$TRANSACTION_CHANGE_SET_OBSERVED_STATE_HASH" || return 1
        [ -n "$TRANSACTION_CHANGE_SET_HEALTH_CHECKS" ] || return 1
        [ "$TRANSACTION_CHANGE_SET_POLICY" = "local_auto" ] || return 1
        transaction_write_atomic "$transaction_path/change_set_id" "$TRANSACTION_CHANGE_SET_ID" || return 1
        transaction_write_atomic "$transaction_path/change_set_device_id" "$TRANSACTION_CHANGE_SET_DEVICE_ID" || return 1
        transaction_write_atomic "$transaction_path/change_set_plan_hash" "$TRANSACTION_CHANGE_SET_PLAN_HASH" || return 1
        transaction_write_atomic "$transaction_path/change_set_generation" "$transaction_generation" || return 1
        transaction_write_atomic "$transaction_path/change_set_operation_id" "$TRANSACTION_CHANGE_SET_OPERATION_ID" || return 1
        transaction_write_atomic "$transaction_path/change_set_commands" "$TRANSACTION_CHANGE_SET_COMMANDS" || return 1
        transaction_write_atomic "$transaction_path/change_set_observed_state_hash" "$TRANSACTION_CHANGE_SET_OBSERVED_STATE_HASH" || return 1
        transaction_write_atomic "$transaction_path/change_set_health_checks" "$TRANSACTION_CHANGE_SET_HEALTH_CHECKS" || return 1
        transaction_write_atomic "$transaction_path/change_set_confirmation_policy" "$TRANSACTION_CHANGE_SET_POLICY" || return 1
    else
        rm -f "$transaction_path/change_set_id" "$transaction_path/change_set_device_id" "$transaction_path/change_set_plan_hash" "$transaction_path/change_set_generation" "$transaction_path/change_set_operation_id" "$transaction_path/change_set_commands" "$transaction_path/change_set_observed_state_hash" "$transaction_path/change_set_health_checks" "$transaction_path/change_set_confirmation_policy"
    fi
    transaction_write_atomic "$transaction_path/state" APPLYING || return 1
    transaction_write_atomic "$NERVE_TRANSACTION_ROOT/active" "$transaction_id" || return 1
}

transaction_mark_pending() {
    local transaction_config="$1"
    local transaction_id="$2"
    transaction_valid_config "$transaction_config" || return 1
    transaction_valid_id "$transaction_id" || return 1
    transaction_write_atomic "$(transaction_dir "$transaction_id")/state" PENDING_CONFIRM
}

transaction_commit() {
    local transaction_config="$1"
    local transaction_id="$2"
    local transaction_hash_file="$3"
    local transaction_hash="$4"
    local transaction_path
    transaction_valid_config "$transaction_config" || return 1
    transaction_valid_id "$transaction_id" || return 1
    transaction_path=$(transaction_dir "$transaction_id")
    transaction_write_atomic "$transaction_path/state" COMMITTED || return 1
    transaction_write_atomic "$transaction_hash_file" "$transaction_hash" || return 1
    rm -f "$NERVE_TRANSACTION_ROOT/active" "$transaction_path/backup" "$transaction_path/backup_exists"
    transaction_write_atomic "$NERVE_TRANSACTION_ROOT/last" "$transaction_id"
}

transaction_status_json() {
    local transaction_id transaction_path transaction_config transaction_state transaction_plan_hash transaction_generation
    transaction_id=$(cat "$NERVE_OPERATION_STATUS_FILE" 2>/dev/null || true)
    if [ -n "$transaction_id" ] && ! transaction_valid_id "$transaction_id"; then
        transaction_id=""
    fi
    if [ -n "$transaction_id" ] && [ ! -f "$(transaction_dir "$transaction_id")/state" ]; then
        transaction_id=""
    fi
    [ -n "$transaction_id" ] || transaction_id=$(cat "$NERVE_TRANSACTION_ROOT/active" 2>/dev/null || true)
    [ -n "$transaction_id" ] || transaction_id=$(cat "$NERVE_TRANSACTION_ROOT/last" 2>/dev/null || true)
    if ! transaction_valid_id "$transaction_id"; then
        printf '{}'
        return 0
    fi
    transaction_path=$(transaction_dir "$transaction_id")
    if [ -f "$transaction_path/change_set_device_id" ]; then
        printf '{}'
        return 0
    fi
    transaction_config=$(cat "$transaction_path/config" 2>/dev/null || true)
    transaction_state=$(cat "$transaction_path/state" 2>/dev/null || true)
    if ! transaction_valid_config "$transaction_config"; then
        printf '{}'
        return 0
    fi
    if [ -f "$transaction_path/plan_hash" ]; then
        transaction_plan_hash=$(cat "$transaction_path/plan_hash" 2>/dev/null || true)
    else
        transaction_plan_hash=$(cat "$NERVE_TRANSACTION_ROOT/operation_${transaction_config}.hash" 2>/dev/null || true)
    fi
    transaction_generation=$(cat "$transaction_path/generation" 2>/dev/null || true)
    if [ -n "$transaction_generation" ] && ! transaction_valid_generation "$transaction_generation"; then
        transaction_generation=""
    fi
    if transaction_valid_plan_hash "$transaction_plan_hash"; then
        if [ -n "$transaction_generation" ] && [ "$transaction_generation" -gt 0 ] 2>/dev/null; then
            printf '{"id":"%s","config":"%s","state":"%s","plan_hash":"%s","generation":%s}' "$transaction_id" "$transaction_config" "$transaction_state" "$transaction_plan_hash" "$transaction_generation"
        else
            printf '{"id":"%s","config":"%s","state":"%s","plan_hash":"%s"}' "$transaction_id" "$transaction_config" "$transaction_state" "$transaction_plan_hash"
        fi
    elif [ -n "$transaction_generation" ] && [ "$transaction_generation" -gt 0 ] 2>/dev/null; then
        printf '{"id":"%s","config":"%s","state":"%s","generation":%s}' "$transaction_id" "$transaction_config" "$transaction_state" "$transaction_generation"
    else
        printf '{"id":"%s","config":"%s","state":"%s"}' "$transaction_id" "$transaction_config" "$transaction_state"
    fi
}

operation_valid_name() {
    case "$1" in
        ''|*[!A-Za-z0-9_-]*) return 1 ;;
    esac
}

operation_valid_section() {
    printf '%s\n' "$1" | grep -Eq '^([A-Za-z0-9_-]+|@[A-Za-z0-9_-]+\[-?[0-9]+\])$'
}

operation_valid_mac() {
    case "$1" in
        [A-Fa-f0-9][A-Fa-f0-9]:[A-Fa-f0-9][A-Fa-f0-9]:[A-Fa-f0-9][A-Fa-f0-9]:[A-Fa-f0-9][A-Fa-f0-9]:[A-Fa-f0-9][A-Fa-f0-9]:[A-Fa-f0-9][A-Fa-f0-9]) return 0 ;;
    esac
    return 1
}

operation_valid_ipv4() {
    local operation_ip="$1"
    local operation_old_ifs="$IFS"
    local operation_octet
    local operation_count=0
    case "$operation_ip" in
        ''|*[!0-9.]*) return 1 ;;
    esac
    IFS=.
    set -- $operation_ip
    IFS="$operation_old_ifs"
    [ "$#" -eq 4 ] || return 1
    for operation_octet in "$@"; do
        case "$operation_octet" in
            ''|*[!0-9]*) return 1 ;;
        esac
        [ "$operation_octet" -le 255 ] 2>/dev/null || return 1
        operation_count=$((operation_count + 1))
    done
    [ "$operation_count" -eq 4 ]
}

operation_apply_ensure_host() {
    local operation_name="$1"
    local operation_macs_raw="$2"
    local operation_ip="$3"
    local operation_macs operation_mac operation_host_ref operation_mac_count

    [ -n "$operation_name" ] && [ "${#operation_name}" -le 128 ] || return 1
    if printf '%s' "$operation_name" | grep -q '[[:cntrl:]]'; then
        return 1
    fi
    operation_valid_ipv4 "$operation_ip" || return 1
    operation_macs=$(printf '%s' "$operation_macs_raw" | sed "s/'//g; s/\"//g")
    [ -n "$operation_macs" ] || return 1
    for operation_mac in $operation_macs; do
        operation_valid_mac "$operation_mac" || return 1
    done

    operation_host_ref=""
    for operation_mac in $operation_macs; do
        operation_host_ref=$(uci show dhcp 2>/dev/null | grep -i -F "$operation_mac" | grep -F ".mac=" | cut -d= -f1 | cut -d. -f2 | head -n 1)
        [ -n "$operation_host_ref" ] && break
    done
    if [ -z "$operation_host_ref" ]; then
        operation_host_ref=$(
            uci show dhcp 2>/dev/null | grep -F ".ip=" | while IFS= read -r operation_host_line; do
                operation_host_path=${operation_host_line%%=*}
                operation_host_value=${operation_host_line#*=}
                operation_host_value=$(printf '%s' "$operation_host_value" | sed "s/^'//; s/'$//")
                if [ "$operation_host_value" = "$operation_ip" ]; then
                    operation_host_path=${operation_host_path#dhcp.}
                    operation_host_path=${operation_host_path%.ip}
                    printf '%s\n' "$operation_host_path"
                    break
                fi
            done
        )
    fi
    if [ -z "$operation_host_ref" ]; then
        uci add dhcp host
        operation_host_ref=@host[-1]
    fi

    uci set "dhcp.$operation_host_ref.name=$operation_name"
    uci -q delete "dhcp.$operation_host_ref.mac" || true
    operation_mac_count=0
    for operation_mac in $operation_macs; do
        operation_mac_count=$((operation_mac_count + 1))
    done
    if [ "$operation_mac_count" -eq 1 ]; then
        uci set "dhcp.$operation_host_ref.mac=$operation_macs"
    else
        for operation_mac in $operation_macs; do
            uci add_list "dhcp.$operation_host_ref.mac=$operation_mac"
        done
    fi
    uci set "dhcp.$operation_host_ref.ip=$operation_ip"
}

operation_apply_command() {
    local operation_action="$1"
    local operation_config="$2"
    local operation_section="$3"
    local operation_option="$4"
    local operation_value="$5"
    local operation_path

    transaction_valid_config "$operation_config" || return 1
    case "$operation_action" in
        set)
            operation_valid_section "$operation_section" || return 1
            if [ -n "$operation_option" ]; then
                operation_valid_name "$operation_option" || return 1
                operation_path="$operation_config.$operation_section.$operation_option"
            else
                operation_path="$operation_config.$operation_section"
            fi
            uci set "$operation_path=$operation_value"
            ;;
        delete)
            operation_valid_section "$operation_section" || return 1
            if [ -n "$operation_option" ]; then
                operation_valid_name "$operation_option" || return 1
                operation_path="$operation_config.$operation_section.$operation_option"
            else
                operation_path="$operation_config.$operation_section"
            fi
            uci -q delete "$operation_path" || true
            ;;
        add_list|del_list)
            operation_valid_section "$operation_section" || return 1
            operation_valid_name "$operation_option" || return 1
            operation_path="$operation_config.$operation_section.$operation_option"
            if [ "$operation_action" = "add_list" ]; then
                uci add_list "$operation_path=$operation_value"
            else
                uci del_list "$operation_path=$operation_value"
            fi
            ;;
        add)
            operation_valid_name "$operation_value" || return 1
            uci add "$operation_config" "$operation_value"
            ;;
        delete_all)
            [ "$operation_config" = "firewall" ] || return 1
            [ "$operation_section" = "redirect" ] && [ -z "$operation_option" ] && [ -z "$operation_value" ] || return 1
            while uci -q delete "$operation_config.@$operation_section[0]"; do :; done
            ;;
        ensure_host)
            [ "$operation_config" = "dhcp" ] || return 1
            operation_apply_ensure_host "$operation_section" "$operation_option" "$operation_value"
            ;;
        rename)
            operation_valid_section "$operation_section" || return 1
            if [ -n "$operation_option" ]; then
                operation_valid_name "$operation_option" || return 1
                operation_path="$operation_config.$operation_section.$operation_option"
            else
                operation_path="$operation_config.$operation_section"
            fi
            operation_valid_name "$operation_value" || return 1
            uci rename "$operation_path=$operation_value"
            ;;
        *)
            return 1
            ;;
    esac
}

operation_validate_command() {
    local operation_action="$1"
    local operation_config="$2"
    local operation_section="$3"
    local operation_option="$4"
    local operation_value="$5"
    local operation_macs operation_mac operation_mac_count

    transaction_valid_config "$operation_config" || return 1
    case "$operation_action" in
        set|delete|rename)
            operation_valid_section "$operation_section" || return 1
            if [ -n "$operation_option" ]; then
                operation_valid_name "$operation_option" || return 1
            fi
            if [ "$operation_action" = "rename" ]; then
                operation_valid_name "$operation_value" || return 1
            fi
            ;;
        add_list|del_list)
            operation_valid_section "$operation_section" || return 1
            operation_valid_name "$operation_option" || return 1
            ;;
        add)
            operation_valid_name "$operation_value" || return 1
            ;;
        delete_all)
            [ "$operation_config" = "firewall" ] && [ "$operation_section" = "redirect" ] && [ -z "$operation_option" ] && [ -z "$operation_value" ] || return 1
            ;;
        ensure_host)
            [ "$operation_config" = "dhcp" ] || return 1
            [ -n "$operation_section" ] && [ "${#operation_section}" -le 128 ] || return 1
            if printf '%s' "$operation_section" | grep -q '[[:cntrl:]]'; then
                return 1
            fi
            operation_valid_ipv4 "$operation_value" || return 1
            operation_macs=$(printf '%s' "$operation_option" | sed "s/'//g; s/\"//g")
            [ -n "$operation_macs" ] || return 1
            operation_mac_count=0
            for operation_mac in $operation_macs; do
                operation_valid_mac "$operation_mac" || return 1
                operation_mac_count=$((operation_mac_count + 1))
            done
            [ "$operation_mac_count" -gt 0 ] || return 1
            ;;
        *)
            return 1
            ;;
    esac
}

operation_validate_health_checks() {
    local operation_json="$1"
    local operation_target_count operation_index operation_target
    operation_target_count=$(printf '%s' "$operation_json" | jsonfilter -e '@.health_checks[@]' 2>/dev/null | wc -l 2>/dev/null || echo 0)
    operation_index=0
    while [ "$operation_index" -lt "$operation_target_count" ]; do
        operation_target=$(printf '%s' "$operation_json" | jsonfilter -e "@.health_checks[$operation_index]" 2>/dev/null)
        case "$operation_target" in
            ''|*[!A-Za-z0-9.:-]*) return 1 ;;
        esac
        [ "${#operation_target}" -le 253 ] || return 1
        operation_index=$((operation_index + 1))
    done
}

operation_validate_payload() {
    local operation_json="$1"
    local operation_config operation_count operation_index
    local operation_action operation_command_config operation_section operation_option operation_value
    operation_config=$(printf '%s' "$operation_json" | jsonfilter -e '@.config' 2>/dev/null)
    operation_count=$(printf '%s' "$operation_json" | jsonfilter -e '@.commands[@]' 2>/dev/null | wc -l 2>/dev/null || echo 0)
    [ "$operation_count" -gt 0 ] || return 1
    operation_index=0
    while [ "$operation_index" -lt "$operation_count" ]; do
        operation_action=$(printf '%s' "$operation_json" | jsonfilter -e "@.commands[$operation_index].action" 2>/dev/null)
        operation_command_config=$(printf '%s' "$operation_json" | jsonfilter -e "@.commands[$operation_index].config" 2>/dev/null)
        operation_section=$(printf '%s' "$operation_json" | jsonfilter -e "@.commands[$operation_index].section" 2>/dev/null)
        operation_option=$(printf '%s' "$operation_json" | jsonfilter -e "@.commands[$operation_index].option" 2>/dev/null)
        operation_value=$(printf '%s' "$operation_json" | jsonfilter -e "@.commands[$operation_index].value" 2>/dev/null)
        [ "$operation_command_config" = "$operation_config" ] || return 1
        operation_validate_command "$operation_action" "$operation_config" "$operation_section" "$operation_option" "$operation_value" || return 1
        operation_index=$((operation_index + 1))
    done
    operation_validate_health_checks "$operation_json"
}

operation_health_check() {
    local operation_json="$1"
    local operation_config="$2"
    local operation_target_count operation_index operation_target
    operation_target_count=$(printf '%s' "$operation_json" | jsonfilter -e '@.health_checks[@]' 2>/dev/null | wc -l 2>/dev/null || echo 0)
    if [ "$operation_config" = "network" ] && [ "$operation_target_count" -eq 0 ]; then
        return 1
    fi
    operation_index=0
    while [ "$operation_index" -lt "$operation_target_count" ]; do
        operation_target=$(printf '%s' "$operation_json" | jsonfilter -e "@.health_checks[$operation_index]" 2>/dev/null)
        [ -n "$operation_target" ] || return 1
        ping -c 1 -W 2 "$operation_target" >/dev/null 2>&1 || return 1
        operation_index=$((operation_index + 1))
    done
    return 0
}

apply_pending_operation() {
    local operation_json="$1"
    local operation_id operation_config operation_hash operation_generation operation_count operation_index
    local operation_action operation_command_config operation_section operation_option operation_value
    local operation_observed_state_hash observed_state_hash transaction_id
    local transaction_status operation_status transaction_path transaction_state
    operation_id=$(printf '%s' "$operation_json" | jsonfilter -e '@.operation_id' 2>/dev/null)
    operation_config=$(printf '%s' "$operation_json" | jsonfilter -e '@.config' 2>/dev/null)
    operation_hash=$(printf '%s' "$operation_json" | jsonfilter -e '@.plan_hash' 2>/dev/null)
    operation_generation=$(printf '%s' "$operation_json" | jsonfilter -e '@.generation' 2>/dev/null)
    operation_observed_state_hash=$(printf '%s' "$operation_json" | jsonfilter -e '@.observed_state_hash' 2>/dev/null)
    transaction_id="$operation_id"
    if [ "${TRANSACTION_CHANGE_SET:-0}" = "1" ]; then
        transaction_id="$TRANSACTION_CHANGE_SET_ID"
    fi
    [ -n "$operation_generation" ] || operation_generation=0
    operation_count=$(printf '%s' "$operation_json" | jsonfilter -e '@.commands[@]' 2>/dev/null | wc -l 2>/dev/null || echo 0)
    transaction_valid_id "$operation_id" || return 1
    transaction_valid_plan_hash "$operation_hash" || return 1
    transaction_valid_generation "$operation_generation" || return 1
    transaction_valid_config "$operation_config" || return 1
    [ "$operation_count" -gt 0 ] || return 1
	operation_validate_payload "$operation_json" || return 1
	if [ -n "$operation_observed_state_hash" ]; then
		transaction_valid_plan_hash "$operation_observed_state_hash" || return 1
        transaction_path=$(transaction_dir "$transaction_id")
		transaction_state=$(cat "$transaction_path/state" 2>/dev/null || true)
		if [ "$transaction_state" != "COMMITTED" ]; then
			observed_state_hash=$(uci show "$operation_config" 2>&1 | sha256sum | awk '{print $1}')
			[ "$observed_state_hash" = "$operation_observed_state_hash" ] || return 1
		fi
	fi

    transaction_begin "$operation_config" "$transaction_id" "$operation_generation" "$operation_hash"
    transaction_status=$?
    if [ "$transaction_status" -eq 10 ]; then
        return 0
    fi
    [ "$transaction_status" -eq 0 ] || return 1

    if [ "${TRANSACTION_CHANGE_SET:-0}" = "1" ]; then
        change_set_status_write "$TRANSACTION_CHANGE_SET_ID" "$TRANSACTION_CHANGE_SET_DEVICE_ID" "$TRANSACTION_CHANGE_SET_PLAN_HASH" "$TRANSACTION_CHANGE_SET_GENERATION" PREPARED || {
            transaction_mark_recovery_required "$transaction_path" "changeset status could not be persisted" || true
            return 1
        }
        change_set_status_write "$TRANSACTION_CHANGE_SET_ID" "$TRANSACTION_CHANGE_SET_DEVICE_ID" "$TRANSACTION_CHANGE_SET_PLAN_HASH" "$TRANSACTION_CHANGE_SET_GENERATION" APPLYING || {
            transaction_mark_recovery_required "$transaction_path" "changeset applying status could not be persisted" || true
            return 1
        }
    fi

    (
        set -e
        operation_index=0
        while [ "$operation_index" -lt "$operation_count" ]; do
            operation_action=$(printf '%s' "$operation_json" | jsonfilter -e "@.commands[$operation_index].action" 2>/dev/null)
            operation_command_config=$(printf '%s' "$operation_json" | jsonfilter -e "@.commands[$operation_index].config" 2>/dev/null)
            operation_section=$(printf '%s' "$operation_json" | jsonfilter -e "@.commands[$operation_index].section" 2>/dev/null)
            operation_option=$(printf '%s' "$operation_json" | jsonfilter -e "@.commands[$operation_index].option" 2>/dev/null)
            operation_value=$(printf '%s' "$operation_json" | jsonfilter -e "@.commands[$operation_index].value" 2>/dev/null)
            [ "$operation_command_config" = "$operation_config" ] || exit 1
            operation_apply_command "$operation_action" "$operation_config" "$operation_section" "$operation_option" "$operation_value"
            operation_index=$((operation_index + 1))
        done
        uci commit "$operation_config"
    )
    operation_status=$?
    if [ "$operation_status" -ne 0 ]; then
        transaction_recover_pending || true
        if [ "${TRANSACTION_CHANGE_SET:-0}" != "1" ]; then
            transaction_write_atomic "$NERVE_OPERATION_STATUS_FILE" "$operation_id" || true
        fi
        return 1
    fi

    transaction_mark_pending "$operation_config" "$transaction_id" || {
        transaction_recover_pending || true
        return 1
    }
    if [ "${TRANSACTION_CHANGE_SET:-0}" = "1" ]; then
        change_set_status_write "$TRANSACTION_CHANGE_SET_ID" "$TRANSACTION_CHANGE_SET_DEVICE_ID" "$TRANSACTION_CHANGE_SET_PLAN_HASH" "$TRANSACTION_CHANGE_SET_GENERATION" PENDING_CONFIRM || {
            transaction_recover_pending || true
            return 1
        }
    fi
    transaction_restart_config "$operation_config" || {
        transaction_recover_pending || true
        return 1
    }
    uci show "$operation_config" >/dev/null 2>&1 || {
        transaction_recover_pending || true
        return 1
    }
    operation_health_check "$operation_json" "$operation_config" || {
        logger -t agent "TRANSACTION_HEALTH_FAILED: restoring $operation_config for $operation_id"
        transaction_recover_pending || true
        return 1
    }
    transaction_commit "$operation_config" "$transaction_id" "$NERVE_TRANSACTION_ROOT/operation_${operation_config}.hash" "$operation_hash" || {
        transaction_recover_pending || true
        if [ "${TRANSACTION_CHANGE_SET:-0}" != "1" ]; then
            transaction_write_atomic "$NERVE_OPERATION_STATUS_FILE" "$operation_id" || true
        fi
        return 1
    }
    if [ "${TRANSACTION_CHANGE_SET:-0}" != "1" ]; then
        transaction_write_atomic "$NERVE_OPERATION_STATUS_FILE" "$operation_id" || return 1
    fi
    logger -t agent "TRANSACTION_COMMITTED: $operation_config operation $operation_id"
    return 0
}

apply_pending_change_set_single() {
    local change_set_json="$1"
    local change_set_id change_set_device_id change_set_plan_hash change_set_generation
    local change_set_policy operation_count operation_json operation_commands operation_health_checks operation_health_checks_hash operation_id
    local operation_config operation_observed_state_hash transaction_path transaction_state stored_operation_id
    local existing_status existing_id existing_plan_hash existing_generation existing_state existing_device_id
    local expected_device_id actual_device_id observed_state_hash
    local failure_state failure_detail

    change_set_id=$(printf '%s' "$change_set_json" | jsonfilter -e '@.change_set_id' 2>/dev/null)
    change_set_device_id=$(printf '%s' "$change_set_json" | jsonfilter -e '@.device_id' 2>/dev/null)
    change_set_plan_hash=$(printf '%s' "$change_set_json" | jsonfilter -e '@.plan_hash' 2>/dev/null)
    change_set_generation=$(printf '%s' "$change_set_json" | jsonfilter -e '@.generation' 2>/dev/null)
    change_set_policy=$(printf '%s' "$change_set_json" | jsonfilter -e '@.confirmation_policy' 2>/dev/null)
    operation_count=$(printf '%s' "$change_set_json" | jsonfilter -e '@.operations[@]' 2>/dev/null | wc -l 2>/dev/null || echo 0)
    operation_id=$(printf '%s' "$change_set_json" | jsonfilter -e '@.operations[0].operation_id' 2>/dev/null)
    operation_config=$(printf '%s' "$change_set_json" | jsonfilter -e '@.operations[0].config' 2>/dev/null)
    operation_observed_state_hash=$(printf '%s' "$change_set_json" | jsonfilter -e '@.operations[0].observed_state_hash' 2>/dev/null)
    operation_commands=$(printf '%s' "$change_set_json" | jsonfilter -e '@.operations[0].commands' 2>/dev/null)
    operation_health_checks=$(printf '%s' "$change_set_json" | jsonfilter -e '@.health_checks' 2>/dev/null)
    operation_health_checks_hash="$operation_health_checks"
    case "$operation_health_checks" in
        '')
            operation_health_checks="[]"
            operation_health_checks_hash="null"
            ;;
        null)
            operation_health_checks="[]"
            operation_health_checks_hash="null"
            ;;
        \[*\]) ;;
        *)
            change_set_reject "invalid changeset health check list"
            return 1
            ;;
    esac

    transaction_valid_id "$change_set_id" || return 1
    change_set_valid_device_id "$change_set_device_id" || return 1
    transaction_valid_plan_hash "$change_set_plan_hash" || return 1
    transaction_valid_generation "$change_set_generation" || return 1
    if ! [ "$change_set_generation" -gt 0 ] 2>/dev/null; then
        change_set_reject "invalid changeset generation"
        return 1
    fi

    transaction_path=$(transaction_dir "$change_set_id")
    transaction_state=$(cat "$transaction_path/state" 2>/dev/null || true)
    if [ "$change_set_policy" != "local_auto" ]; then
        change_set_reject "unsupported changeset confirmation policy"
        return 1
    fi
    if ! [ "$operation_count" -eq 1 ]; then
        change_set_reject "changeset must contain exactly one operation"
        return 1
    fi
    if ! transaction_valid_id "$operation_id"; then
        change_set_reject "invalid changeset operation identity"
        return 1
    fi
    if [ "$operation_config" != "system" ]; then
        change_set_reject "unsupported changeset namespace"
        return 1
    fi
    if ! transaction_valid_plan_hash "$operation_observed_state_hash"; then
        change_set_reject "invalid observed system state hash"
        return 1
    fi
    case "$operation_commands" in
        \[*\]) ;;
        *)
            change_set_reject "invalid changeset command list"
            return 1
            ;;
    esac

    expected_device_id=$(printf '%s' "$DEVICE_ID" | tr '[:lower:]' '[:upper:]')
    actual_device_id=$(printf '%s' "$change_set_device_id" | tr '[:lower:]' '[:upper:]')
    if [ "$expected_device_id" != "$actual_device_id" ]; then
        change_set_reject "changeset device identity mismatch"
        return 1
    fi
    if [ "$(change_set_content_plan_hash "$operation_commands" "$operation_health_checks_hash" "$operation_observed_state_hash")" != "$change_set_plan_hash" ]; then
        change_set_reject "changeset plan hash does not match content"
        return 1
    fi

    stored_operation_id=$(cat "$transaction_path/change_set_operation_id" 2>/dev/null || true)
    if [ -n "$stored_operation_id" ] && [ "$stored_operation_id" != "$operation_id" ]; then
        change_set_reject "changeset operation identity does not match"
        return 1
    fi

    existing_status=$(change_set_status_json)
    existing_id=$(printf '%s' "$existing_status" | jsonfilter -e '@.change_set_id' 2>/dev/null || true)
    existing_plan_hash=$(printf '%s' "$existing_status" | jsonfilter -e '@.plan_hash' 2>/dev/null || true)
    existing_generation=$(printf '%s' "$existing_status" | jsonfilter -e '@.generation' 2>/dev/null || true)
    existing_state=$(printf '%s' "$existing_status" | jsonfilter -e '@.state' 2>/dev/null || true)
    if [ "$existing_id" = "$change_set_id" ]; then
        existing_device_id=$(printf '%s' "$existing_status" | jsonfilter -e '@.device_id' 2>/dev/null || true)
        if [ "$(printf '%s' "$existing_device_id" | tr '[:lower:]' '[:upper:]')" != "$(printf '%s' "$change_set_device_id" | tr '[:lower:]' '[:upper:]')" ] ||
            [ "$existing_plan_hash" != "$change_set_plan_hash" ] ||
            [ "$existing_generation" != "$change_set_generation" ]; then
            logger -t agent "TRANSACTION_REJECTED: terminal changeset identity mismatch for $change_set_id"
            return 1
        fi
        case "$existing_state" in
            COMMITTED|RESTORED|REJECTED)
                case "$transaction_state" in
                    COMMITTED|RESTORED)
                        transaction_change_set_identity_matches "$transaction_path" "$change_set_id" "$change_set_device_id" "$change_set_generation" "$change_set_plan_hash" "$operation_id" "$operation_commands" "$operation_observed_state_hash" "$operation_health_checks" "$change_set_policy" || return 1
                        ;;
                esac
                return 0
                ;;
        esac
    fi

    case "$transaction_state" in
        COMMITTED|RESTORED)
            if transaction_change_set_identity_matches "$transaction_path" "$change_set_id" "$change_set_device_id" "$change_set_generation" "$change_set_plan_hash" "$operation_id" "$operation_commands" "$operation_observed_state_hash" "$operation_health_checks" "$change_set_policy"; then
                transaction_publish_change_set_status "$change_set_id" || true
                return 0
            fi
            change_set_status_write "$change_set_id" "$change_set_device_id" "$change_set_plan_hash" "$change_set_generation" REJECTED "committed changeset identity does not match" || true
            return 1
            ;;
    esac

    if ! change_set_generation_allowed "$change_set_id" "$change_set_generation"; then
        change_set_reject "changeset generation is stale or device requires recovery"
        return 1
    fi

    observed_state_hash=$(uci show system 2>&1 | sha256sum | awk '{print $1}')
    if [ "$observed_state_hash" != "$operation_observed_state_hash" ]; then
        change_set_status_write "$change_set_id" "$change_set_device_id" "$change_set_plan_hash" "$change_set_generation" REJECTED "observed system state precondition failed" || true
        return 1
    fi

    operation_json=$(printf '{"operation_id":"%s","plan_hash":"%s","generation":%s,"config":"%s","commands":%s,"health_checks":%s,"auto_confirm":true,"observed_state_hash":"%s"}' \
        "$operation_id" "$change_set_plan_hash" "$change_set_generation" "$operation_config" "$operation_commands" "$operation_health_checks" "$operation_observed_state_hash")
    if ! operation_validate_payload "$operation_json"; then
        change_set_reject "invalid changeset command or health check"
        return 1
    fi
    TRANSACTION_CHANGE_SET=1
    TRANSACTION_CHANGE_SET_ID="$change_set_id"
    TRANSACTION_CHANGE_SET_DEVICE_ID="$change_set_device_id"
    TRANSACTION_CHANGE_SET_PLAN_HASH="$change_set_plan_hash"
    TRANSACTION_CHANGE_SET_GENERATION="$change_set_generation"
    TRANSACTION_CHANGE_SET_OPERATION_ID="$operation_id"
    TRANSACTION_CHANGE_SET_COMMANDS="$operation_commands"
    TRANSACTION_CHANGE_SET_OBSERVED_STATE_HASH="$operation_observed_state_hash"
    TRANSACTION_CHANGE_SET_HEALTH_CHECKS="$operation_health_checks"
    TRANSACTION_CHANGE_SET_POLICY="$change_set_policy"
    if apply_pending_operation "$operation_json"; then
        TRANSACTION_CHANGE_SET=0
        change_set_status_write "$change_set_id" "$change_set_device_id" "$change_set_plan_hash" "$change_set_generation" COMMITTED || return 1
        return 0
    fi
    TRANSACTION_CHANGE_SET=0

    failure_state=$(cat "$transaction_path/state" 2>/dev/null || true)
    case "$failure_state" in
        COMMITTED) failure_state=COMMITTED ;;
        RECOVERY_REQUIRED) failure_state=RECOVERY_REQUIRED ;;
        RESTORED) failure_state=RESTORED ;;
        ROLLING_BACK|APPLYING|PENDING_CONFIRM) failure_state=RECOVERY_REQUIRED ;;
        *) failure_state=REJECTED ;;
    esac
    if [ "$failure_state" = "COMMITTED" ]; then
        change_set_status_write "$change_set_id" "$change_set_device_id" "$change_set_plan_hash" "$change_set_generation" COMMITTED || true
        return 0
    fi
    failure_detail=$(cat "$transaction_path/failure" 2>/dev/null || true)
    [ -n "$failure_detail" ] || failure_detail="changeset execution failed"
    change_set_status_write "$change_set_id" "$change_set_device_id" "$change_set_plan_hash" "$change_set_generation" "$failure_state" "$failure_detail" || true
    return 1
}

apply_pending_change_set() {
    local change_set_json="$1"
    local operation_count operation_config
    operation_count=$(printf '%s' "$change_set_json" | jsonfilter -e '@.operations[@]' 2>/dev/null | wc -l 2>/dev/null || echo 0)
    operation_config=$(printf '%s' "$change_set_json" | jsonfilter -e '@.operations[0].config' 2>/dev/null)
    if [ "$operation_count" -eq 1 ] && [ "$operation_config" = system ]; then
        apply_pending_change_set_single "$change_set_json"
        return $?
    fi
    apply_pending_change_set_multi "$change_set_json"
}

apply_pending_change_set_multi() {
    local change_set_json="$1" change_set_id change_set_device_id change_set_plan_hash
    local change_set_generation change_set_policy operation_count operation_index
    local operation_id operation_config operation_commands operation_observed operation_json
    local operation_health_checks operation_state_hash namespace namespaces=""
    local operation_action command_config section option value observed_state_hash
    local transaction_path transaction_state failure_detail health_checks health_target_count
    local order namespace_seen

    change_set_id=$(printf '%s' "$change_set_json" | jsonfilter -e '@.change_set_id' 2>/dev/null)
    change_set_device_id=$(printf '%s' "$change_set_json" | jsonfilter -e '@.device_id' 2>/dev/null)
    change_set_plan_hash=$(printf '%s' "$change_set_json" | jsonfilter -e '@.plan_hash' 2>/dev/null)
    change_set_generation=$(printf '%s' "$change_set_json" | jsonfilter -e '@.generation' 2>/dev/null)
    change_set_policy=$(printf '%s' "$change_set_json" | jsonfilter -e '@.confirmation_policy' 2>/dev/null)
    operation_count=$(printf '%s' "$change_set_json" | jsonfilter -e '@.operations[@]' 2>/dev/null | wc -l 2>/dev/null || echo 0)
    health_checks=$(printf '%s' "$change_set_json" | jsonfilter -e '@.health_checks' 2>/dev/null)
    case "$health_checks" in ''|null) health_checks="[]" ;; \[*\]) ;; *) change_set_reject "invalid changeset health check list"; return 1 ;; esac
    transaction_valid_id "$change_set_id" || return 1
    change_set_valid_device_id "$change_set_device_id" || return 1
    transaction_valid_plan_hash "$change_set_plan_hash" || return 1
    transaction_valid_generation "$change_set_generation" || return 1
    [ "$change_set_generation" -gt 0 ] 2>/dev/null || return 1
    [ "$change_set_policy" = local_auto ] || { change_set_reject "unsupported changeset confirmation policy"; return 1; }
    [ "$operation_count" -gt 0 ] || { change_set_reject "changeset must contain an operation"; return 1; }
    [ "$(printf '%s' "$change_set_device_id" | tr '[:lower:]' '[:upper:]')" = "$(printf '%s' "$DEVICE_ID" | tr '[:lower:]' '[:upper:]')" ] || { change_set_reject "changeset device identity mismatch"; return 1; }

    # Validate every operation and every observed-state precondition before UCI is touched.
    for operation_index in $(seq 0 $((operation_count - 1))); do
        operation_id=$(printf '%s' "$change_set_json" | jsonfilter -e "@.operations[$operation_index].operation_id" 2>/dev/null)
        operation_config=$(printf '%s' "$change_set_json" | jsonfilter -e "@.operations[$operation_index].config" 2>/dev/null)
        operation_commands=$(printf '%s' "$change_set_json" | jsonfilter -e "@.operations[$operation_index].commands" 2>/dev/null)
        operation_observed=$(printf '%s' "$change_set_json" | jsonfilter -e "@.operations[$operation_index].observed_state_hash" 2>/dev/null)
        operation_health_checks=$(printf '%s' "$change_set_json" | jsonfilter -e "@.operations[$operation_index].health_checks" 2>/dev/null)
        case "$operation_health_checks" in ''|null) operation_health_checks="[]" ;; \[*\]) ;; *) change_set_reject "invalid operation health check list"; return 1 ;; esac
        transaction_valid_id "$operation_id" || { change_set_reject "invalid changeset operation identity"; return 1; }
        change_set_valid_config "$operation_config" || { change_set_reject "unsupported changeset namespace"; return 1; }
        case "$operation_commands" in \[*\]) ;; *) change_set_reject "invalid changeset command list"; return 1 ;; esac
        transaction_valid_plan_hash "$operation_observed" || { change_set_reject "invalid observed state hash"; return 1; }
        operation_json=$(printf '{"operation_id":"%s","plan_hash":"%s","generation":%s,"config":"%s","commands":%s,"health_checks":%s,"auto_confirm":true,"observed_state_hash":"%s"}' "$operation_id" "$change_set_plan_hash" "$change_set_generation" "$operation_config" "$operation_commands" "$operation_health_checks" "$operation_observed")
        operation_validate_payload "$operation_json" || { change_set_reject "invalid changeset command or health check"; return 1; }
        observed_state_hash=$(uci show "$operation_config" 2>&1 | sha256sum | awk '{print $1}')
        [ "$observed_state_hash" = "$operation_observed" ] || { change_set_reject "observed state precondition failed"; return 1; }
        case " $namespaces " in *" $operation_config "*) ;; *) namespaces=$(printf '%s\n%s' "$namespaces" "$operation_config") ;; esac
    done

    # Namespace order is explicit and stable, independent of delivery order.
    namespaces=$(printf '%s' "$namespaces" | awk 'NF' | sort -u)
    transaction_path=$(transaction_dir "$change_set_id")
    transaction_state=$(cat "$transaction_path/state" 2>/dev/null || true)
    if [ "$transaction_state" = COMMITTED ] || [ "$transaction_state" = RESTORED ]; then
        change_set_transaction_identity_matches_multi "$transaction_path" "$change_set_id" "$change_set_device_id" "$change_set_plan_hash" "$change_set_generation" \
            "$(printf '%s' "$change_set_json" | jsonfilter -e '@.operations')" "$health_checks" "$change_set_policy" || return 1
        return 0
    fi
    [ "$transaction_state" != RECOVERY_REQUIRED ] || return 1
    if ! change_set_generation_allowed "$change_set_id" "$change_set_generation"; then
        change_set_reject "changeset generation is stale or device requires recovery"; return 1
    fi
    if ! change_set_transaction_begin "$change_set_id" "$namespaces"; then
        change_set_reject "changeset journal snapshot failed"; return 1
    fi
    transaction_path=$(transaction_dir "$change_set_id")
    operation_id=$(printf '%s' "$change_set_json" | jsonfilter -e '@.operations[0].operation_id' 2>/dev/null)
    operation_commands=$(printf '%s' "$change_set_json" | jsonfilter -e '@.operations[0].commands' 2>/dev/null)
    operation_observed=$(printf '%s' "$change_set_json" | jsonfilter -e '@.operations[0].observed_state_hash' 2>/dev/null)
    transaction_write_atomic "$transaction_path/change_set_id" "$change_set_id" || return 1
    transaction_write_atomic "$transaction_path/change_set_device_id" "$change_set_device_id" || return 1
    transaction_write_atomic "$transaction_path/change_set_plan_hash" "$change_set_plan_hash" || return 1
    transaction_write_atomic "$transaction_path/change_set_generation" "$change_set_generation" || return 1
    transaction_write_atomic "$transaction_path/change_set_operation_id" "$operation_id" || return 1
    transaction_write_atomic "$transaction_path/change_set_commands" "$operation_commands" || return 1
    transaction_write_atomic "$transaction_path/change_set_observed_state_hash" "$operation_observed" || return 1
    transaction_write_atomic "$transaction_path/change_set_operations" "$(printf '%s' "$change_set_json" | jsonfilter -e '@.operations')" || return 1
    transaction_write_atomic "$transaction_path/change_set_health_checks" "$health_checks" || return 1
    transaction_write_atomic "$transaction_path/change_set_confirmation_policy" "$change_set_policy" || return 1
    change_set_status_write "$change_set_id" "$change_set_device_id" "$change_set_plan_hash" "$change_set_generation" PREPARED || return 1
    change_set_status_write "$change_set_id" "$change_set_device_id" "$change_set_plan_hash" "$change_set_generation" APPLYING || return 1

    for order in system dhcp firewall dropbear sqm; do
        operation_index=0
        while [ "$operation_index" -lt "$operation_count" ]; do
            operation_config=$(printf '%s' "$change_set_json" | jsonfilter -e "@.operations[$operation_index].config" 2>/dev/null)
            if [ "$operation_config" = "$order" ]; then
                operation_commands=$(printf '%s' "$change_set_json" | jsonfilter -e "@.operations[$operation_index].commands" 2>/dev/null)
                operation_id=$(printf '%s' "$change_set_json" | jsonfilter -e "@.operations[$operation_index].operation_id" 2>/dev/null)
                command_count=$(printf '%s' "$change_set_json" | jsonfilter -e "@.operations[$operation_index].commands[@]" 2>/dev/null | wc -l 2>/dev/null || echo 0)
                command_index=0
                while [ "$command_index" -lt "$command_count" ]; do
                    operation_action=$(printf '%s' "$change_set_json" | jsonfilter -e "@.operations[$operation_index].commands[$command_index].action" 2>/dev/null)
                    section=$(printf '%s' "$change_set_json" | jsonfilter -e "@.operations[$operation_index].commands[$command_index].section" 2>/dev/null)
                    option=$(printf '%s' "$change_set_json" | jsonfilter -e "@.operations[$operation_index].commands[$command_index].option" 2>/dev/null)
                    value=$(printf '%s' "$change_set_json" | jsonfilter -e "@.operations[$operation_index].commands[$command_index].value" 2>/dev/null)
                    operation_apply_command "$operation_action" "$operation_config" "$section" "$option" "$value" || { failure_detail="changeset mutation failed"; break 3; }
                    command_index=$((command_index + 1))
                done
                uci commit "$operation_config" || { failure_detail="changeset commit failed"; break 3; }
            fi
            operation_index=$((operation_index + 1))
        done
    done
    [ -z "${failure_detail:-}" ] || { transaction_recover_pending || true; return 1; }
    for namespace in $namespaces; do
        transaction_restart_config "$namespace" || { transaction_recover_pending || true; return 1; }
        uci show "$namespace" >/dev/null 2>&1 || { transaction_recover_pending || true; return 1; }
    done
    operation_health_check "$(printf '{"health_checks":%s}' "$health_checks")" system || { transaction_recover_pending || true; return 1; }
    transaction_write_atomic "$transaction_path/state" COMMITTED || return 1
    rm -f "$NERVE_TRANSACTION_ROOT/active"
    transaction_write_atomic "$NERVE_TRANSACTION_ROOT/last" "$change_set_id" || return 1
    change_set_status_write "$change_set_id" "$change_set_device_id" "$change_set_plan_hash" "$change_set_generation" COMMITTED || return 1
}

# logd is a local dependency, not part of the telemetry heartbeat. On some
# OpenWrt builds logread can remain blocked on the logd socket; running it in
# the telemetry pipeline would then stop the agent before the POST forever.
# Keep the collection bounded and let telemetry continue when logd is stuck.
collect_recent_logs() {
    local log_file="/tmp/nerve-agent-logread.$$"
    local log_pid
    local waited=0

    rm -f "$log_file"
    logread -l 20 >"$log_file" 2>/dev/null &
    log_pid=$!

    while kill -0 "$log_pid" 2>/dev/null; do
        if [ "$waited" -ge 2 ]; then
            kill "$log_pid" 2>/dev/null
            wait "$log_pid" 2>/dev/null
            rm -f "$log_file"
            return 0
        fi
        sleep 1
        waited=$((waited + 1))
    done

    wait "$log_pid" 2>/dev/null
    sed 's/\\/\\\\/g; s/"/\\"/g' "$log_file" | awk '{printf "%s\\n", $0}'
    rm -f "$log_file"
}

if [ "${1:-}" = "--self-test-signature" ]; then
    [ "$#" -eq 4 ] || exit 1
    verify_agent_artifact "$2" "$3" "$4"
    exit $?
fi

if [ "${1:-}" = "--self-test-transaction" ]; then
    transaction_begin wireless self-test-operation || exit 1
    printf '%s\n' self-test-mutated > "$NERVE_CONFIG_ROOT/wireless"
    transaction_mark_pending wireless self-test-operation || exit 1
    transaction_commit wireless self-test-operation "$NERVE_WIFI_HASH_FILE" self-test-hash || exit 1
    exit 0
fi

if [ "${1:-}" = "--self-test-operation" ]; then
    if [ -n "${SELF_TEST_OPERATION_JSON:-}" ]; then
        SELF_TEST_OPERATION="$SELF_TEST_OPERATION_JSON"
    else
        SELF_TEST_OPERATION='{"operation_id":"self-operation","plan_hash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}],"auto_confirm":true}'
    fi
    apply_pending_operation "$SELF_TEST_OPERATION" || exit 1
    exit 0
fi

if [ "${1:-}" = "--self-test-change-set" ]; then
    if [ -n "${SELF_TEST_CHANGE_SET_JSON:-}" ]; then
        SELF_TEST_CHANGE_SET="$SELF_TEST_CHANGE_SET_JSON"
    elif [ -s "$NERVE_TRANSACTION_ROOT/self-test-change-set.json" ]; then
        SELF_TEST_CHANGE_SET=$(cat "$NERVE_TRANSACTION_ROOT/self-test-change-set.json")
    else
        SELF_TEST_CHANGE_SET_DEVICE_ID="${SELF_TEST_CHANGE_SET_DEVICE_ID:-self-test-device}"
        [ -n "$DEVICE_ID" ] || DEVICE_ID="$SELF_TEST_CHANGE_SET_DEVICE_ID"
        SELF_TEST_CHANGE_SET_OBSERVED_HASH=$(uci show system 2>&1 | sha256sum | awk '{print $1}')
        SELF_TEST_CHANGE_SET_PLAN_HASH=$(change_set_content_plan_hash '[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}]' null "$SELF_TEST_CHANGE_SET_OBSERVED_HASH")
        SELF_TEST_CHANGE_SET=$(printf '{"change_set_id":"self-change-set","device_id":"%s","plan_hash":"%s","generation":42,"operations":[{"operation_id":"self-operation-entry","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}],"observed_state_hash":"%s"}],"confirmation_policy":"local_auto"}' \
            "$SELF_TEST_CHANGE_SET_DEVICE_ID" "$SELF_TEST_CHANGE_SET_PLAN_HASH" "$SELF_TEST_CHANGE_SET_OBSERVED_HASH")
        mkdir -p "$NERVE_TRANSACTION_ROOT"
        transaction_write_atomic "$NERVE_TRANSACTION_ROOT/self-test-change-set.json" "$SELF_TEST_CHANGE_SET"
    fi
    apply_pending_change_set "$SELF_TEST_CHANGE_SET" || exit 1
    exit 0
fi

if [ "${1:-}" = "--self-test-status" ]; then
    CHANGE_SET_STATUS_PAYLOAD=$(change_set_status_json)
    if [ "$CHANGE_SET_STATUS_PAYLOAD" = "{}" ]; then
        transaction_status_json
    else
        printf '%s' "$CHANGE_SET_STATUS_PAYLOAD"
    fi
    exit 0
fi

if [ "${1:-}" = "--self-test-change-set-transition" ]; then
    transition_hash=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    rm -f "$NERVE_CHANGE_SET_STATUS_FILE"
    change_set_status_write transition-test self-test-device "$transition_hash" 42 PREPARED || exit 1
    change_set_status_write transition-test self-test-device "$transition_hash" 42 PREPARED || exit 1
    change_set_status_write transition-test self-test-device "$transition_hash" 42 APPLYING || exit 1
    if change_set_status_write transition-test self-test-device "$transition_hash" 42 PREPARED; then
        exit 1
    fi
    exit 0
fi

if [ "${1:-}" = "--self-test-runtime-config" ]; then
    validate_controller_url
    exit $?
fi

if [ "${1:-}" = "--self-test-log-collection" ]; then
    collect_recent_logs
    exit 0
fi

if [ "${1:-}" = "--recover-transactions" ]; then
    transaction_recover_pending
    exit $?
fi

if [ "${1:-}" = "--prune-transactions" ]; then
    transaction_prune_terminal
    exit $?
fi

if ! validate_controller_url; then
    logger -t agent "Runtime controller URL or TLS policy is invalid"
    exit 1
fi
CONTROLLER_HOST=$(printf '%s\n' "$CONTROLLER_URL" | sed -e 's#^[A-Za-z][A-Za-z0-9+.-]*://##' -e 's#/.*$##' -e 's#:[0-9][0-9]*$##')

if [ -z "$DEVICE_ID" ]; then
    # Seed identity once. A bridge MAC can change when a NIC is added later.
    if [ -r /sys/class/net/br-lan/address ]; then
        DEVICE_ID=$(tr '[:lower:]' '[:upper:]' < /sys/class/net/br-lan/address)
    elif [ -r /sys/class/net/eth0/address ]; then
        DEVICE_ID=$(tr '[:lower:]' '[:upper:]' < /sys/class/net/eth0/address)
    fi
    if [ -n "$DEVICE_ID" ]; then
        printf '%s\n' "$DEVICE_ID" > "$DEVICE_ID_FILE"
        chmod 600 "$DEVICE_ID_FILE"
    fi
fi

if [ -z "$BASE_URL" ] || [ -z "$DEVICE_ID" ]; then
    logger -t agent "Runtime configuration or device identity is missing"
    exit 1
fi
CONFIG_URL="$BASE_URL/devices/$DEVICE_ID/config"

# Recover before the first network request. A transaction left in APPLYING or
# PENDING_CONFIRM is never trusted after a process crash or reboot. Recovery
# failure is reported through the next telemetry heartbeat while all later
# destructive work remains fenced by the persistent journal.
TRANSACTION_RECOVERY_BLOCKED=0
if ! transaction_recover_pending; then
    logger -t agent "Persistent transaction recovery failed; telemetry-only mode"
    TRANSACTION_RECOVERY_BLOCKED=1
fi

bootstrap_agent() {
    [ -n "$DEVICE_TOKEN" ] && {
        rm -f "$ENROLLMENT_TOKEN_FILE" "$ENROLLMENT_NONCE_FILE"
        return 0
    }
    enrollment_token=$(cat "$ENROLLMENT_TOKEN_FILE" 2>/dev/null || true)
    if [ -z "$enrollment_token" ]; then
        logger -t agent "Bootstrap failed: site enrollment token is missing"
        return 1
    fi

    enrollment_nonce=$(cat "$ENROLLMENT_NONCE_FILE" 2>/dev/null || true)
    if [ -z "$enrollment_nonce" ]; then
        enrollment_nonce=$(od -An -tx1 -N16 /dev/urandom 2>/dev/null | tr -d ' \n')
        case "$enrollment_nonce" in
            ????????*) ;;
            *) logger -t agent "Bootstrap failed: could not create enrollment nonce"; return 1 ;;
        esac
        agent_secret_write "$ENROLLMENT_NONCE_FILE" "$enrollment_nonce" || return 1
    else
        case "$enrollment_nonce" in
            *[!A-Fa-f0-9]*) logger -t agent "Bootstrap failed: enrollment nonce is invalid"; return 1 ;;
        esac
        [ "${#enrollment_nonce}" -eq 32 ] || {
            logger -t agent "Bootstrap failed: enrollment nonce length is invalid"
            return 1
        }
    fi
    [ "${#enrollment_nonce}" -eq 32 ] || {
        logger -t agent "Bootstrap failed: enrollment nonce length is invalid"
        return 1
    }

    enrollment_arch=$(uname -m 2>/dev/null | sed 's/[^A-Za-z0-9._-]/_/g')
    enrollment_kernel=$(uname -r 2>/dev/null | sed 's/[^A-Za-z0-9._-]/_/g')
    enrollment_payload=$(printf '{"device_id":"%s","nonce":"%s","capabilities":{"device_change_set":{"version":1,"namespaces":["system","dhcp","firewall","dropbear","sqm"],"max_operations":8,"confirmation_policies":["local_auto"]},"architecture":"%s","kernel":"%s"}}' \
        "$DEVICE_ID" "$enrollment_nonce" "$enrollment_arch" "$enrollment_kernel")
    enrollment_response_file="/tmp/nerve-enrollment-response.$$"
    enrollment_http_code=$(controller_curl -m 10 -sS -X POST \
        -H "Content-Type: application/json" \
        -H "X-Site-Enrollment-Token: $enrollment_token" \
        -d "$enrollment_payload" \
        "$BASE_URL/device-enrollment" -w '%{http_code}' -o "$enrollment_response_file" 2>/dev/null || true)
    if [ "$enrollment_http_code" != "200" ] && [ "$enrollment_http_code" != "201" ]; then
        logger -t agent "Bootstrap enrollment failed (HTTP $enrollment_http_code)"
        rm -f "$enrollment_response_file"
        return 1
    fi
    enrolled_device_id=$(jsonfilter -i "$enrollment_response_file" -e '@.data.device_id' 2>/dev/null || true)
    bootstrap_token=$(jsonfilter -i "$enrollment_response_file" -e '@.data.device_token' 2>/dev/null || true)
    rm -f "$enrollment_response_file"
    if [ "$enrolled_device_id" != "$DEVICE_ID" ] || [ -z "$bootstrap_token" ]; then
        logger -t agent "Bootstrap failed: controller returned an invalid enrollment"
        return 1
    fi
    agent_secret_write "$DEVICE_TOKEN_FILE" "$bootstrap_token" || return 1
    DEVICE_TOKEN="$bootstrap_token"
    rm -f "$ENROLLMENT_TOKEN_FILE" "$ENROLLMENT_NONCE_FILE"
    logger -t agent "Device enrolled and token provisioned"
}

if ! command -v curl >/dev/null 2>&1 || ! command -v jsonfilter >/dev/null 2>&1; then
    logger -t agent "Bootstrap prerequisites are unavailable"
    exit 1
fi
if ! bootstrap_agent; then
    exit 1
fi

T_FAILS=0
NEIGHBOR_APS="[]"

while true; do
    # 0. CHECK AUTO-UPDATE
    # The signed artifact is hashed byte-for-byte because runtime settings are
    # stored outside this file.
    AGENT_VERSION=$(sha256sum "$0" | awk '{print $1}')
    LATEST_JSON=$(controller_curl -m 5 -s -X GET -H "X-Device-Token: $DEVICE_TOKEN" "$BASE_URL/agent/latest")
    
    if [ -n "$LATEST_JSON" ]; then
        LATEST_HASH=$(echo "$LATEST_JSON" | jsonfilter -e '@.version_hash' 2>/dev/null)
        LATEST_VERSION_NUMBER=$(echo "$LATEST_JSON" | jsonfilter -e '@.version_number' 2>/dev/null)
        if [ -n "$LATEST_HASH" ] && [ "$LATEST_HASH" != "$AGENT_VERSION" ]; then
            LATEST_SIGNATURE=$(echo "$LATEST_JSON" | jsonfilter -e '@.signature' 2>/dev/null)
            LATEST_SIGNATURE_ALGORITHM=$(echo "$LATEST_JSON" | jsonfilter -e '@.signature_algorithm' 2>/dev/null)
            case "$LATEST_VERSION_NUMBER" in
                ''|*[!0-9]*) logger -t agent "Signed update version is missing or malformed; refusing update"; continue ;;
            esac
            if [ "$LATEST_VERSION_NUMBER" -le "${AGENT_VERSION_NUMBER:-0}" ]; then
                logger -t agent "Signed update is not newer than the installed version; refusing downgrade"
                continue
            fi
            logger -t agent "New agent version found: $LATEST_HASH. Downloading..."
            if controller_curl -m 10 -s -X GET -H "X-Device-Token: $DEVICE_TOKEN" "$BASE_URL/agent/latest/raw" -o "$0.tmp"; then
                TMP_HASH=$(sha256sum "$0.tmp" | awk '{print $1}')
                SIGNATURE_OK=0
                if [ -n "$LATEST_SIGNATURE" ] && [ "$LATEST_SIGNATURE_ALGORITHM" = "Ed25519" ]; then
                    if verify_agent_artifact "$0.tmp" "$LATEST_SIGNATURE" "$AGENT_UPDATE_PUBLIC_KEY"; then
                        SIGNATURE_OK=1
                    fi
                else
                    logger -t agent "Signed update metadata is missing or uses an unsupported algorithm"
                fi
                if [ "$TMP_HASH" = "$LATEST_HASH" ] && [ "$SIGNATURE_OK" = "1" ]; then
                    logger -t agent "Agent downloaded securely. Updating and restarting."
                    if chmod +x "$0.tmp" && cp "$0" "$0.old" && mv "$0.tmp" "$0" && printf '%s\n' "$LATEST_VERSION_NUMBER" > "$AGENT_VERSION_NUMBER_FILE"; then
                        AGENT_VERSION_NUMBER="$LATEST_VERSION_NUMBER"
                        logger -t agent "Agent updated. Reloading in-process to preserve procd respawn budget."
                        # Use exec to re-exec the new script in the same PID.
                        # procd never sees a process exit, so the crash counter is preserved
                        # across self-updates (5 clean exits within 1h would otherwise mark
                        # the instance as crashed and procd would stop respawning it).
                        if ! exec /bin/sh "$0"; then
                            logger -t agent "exec failed; restoring the previous agent"
                            if ! mv "$0.old" "$0"; then
                                logger -t agent "previous agent could not be restored"
                            fi
                            exit 0
                        fi
                    else
                        logger -t agent "Agent replacement failed; preserving the current agent"
                        rm -f "$0.tmp"
                    fi
                else
                    logger -t agent "Hash mismatch on new agent. Aborting update."
                    rm -f "$0.tmp"
                fi
            fi
        fi
    fi
    # 1. INFORMACIÓN BÁSICA DEL SISTEMA
    BOARD=$(ubus call system board 2>/dev/null || echo "{}")
    SYS_INFO=$(ubus call system info 2>/dev/null || echo "{}")

    # Keep this inventory bounded and best-effort: missing OpenWrt packages
    # must not prevent the regular telemetry heartbeat.
    CAP_ARCH=$(uname -m 2>/dev/null || echo unknown)
    CAP_KERNEL=$(uname -r 2>/dev/null || echo unknown)
    CAP_RELEASE=$(echo "$BOARD" | jsonfilter -e '@.release.version' 2>/dev/null || echo unknown)
    CAP_RAM_MB=$(free -m 2>/dev/null | awk 'NR==2 && $2 ~ /^[0-9]+$/ {print $2}' || true)
    CAP_FLASH_MB=$(df -Pm /overlay 2>/dev/null | awk 'NR==2 && $2 ~ /^[0-9]+$/ {print $2}' || true)
    case "$CAP_RAM_MB" in *[!0-9]*|'') CAP_RAM_MB=0 ;; esac
    case "$CAP_FLASH_MB" in *[!0-9]*|'') CAP_FLASH_MB=0 ;; esac
    CAP_INTERFACES=$(ip -o link show 2>/dev/null | awk -F': ' '{print $2}' | cut -d'@' -f1 | head -n 32 | sed 's/.*/"&"/' | join_csv)
    [ -n "$CAP_INTERFACES" ] || CAP_INTERFACES=""
    CAP_RADIOS=$(iwinfo 2>/dev/null | awk '/^[a-zA-Z0-9_.-]+[[:space:]]+ESSID:/ {print $1}' | head -n 16 | sed 's/.*/"&"/' | join_csv)
    [ -n "$CAP_RADIOS" ] || CAP_RADIOS=""
    CAP_WIFI_DEVICES=$(uci -q show wireless 2>/dev/null | awk -F'[.=]' '/=wifi-device/ {print $2}' | head -n 8 | sed 's/.*/"&"/' | join_csv)
    CAP_WIFI_IFACES=$(uci -q show wireless 2>/dev/null | awk -F'[.=]' '/=wifi-iface/ {print $2}' | head -n 16 | sed 's/.*/"&"/' | join_csv)
    [ -n "$CAP_WIFI_DEVICES" ] || CAP_WIFI_DEVICES=""
    [ -n "$CAP_WIFI_IFACES" ] || CAP_WIFI_IFACES=""
    CAP_LOGICAL_NETWORKS=$(uci -q show network 2>/dev/null | awk -F'[.=]' '/\.device=/ {print $1":"$2}' | head -n 16 | awk -F: '{print "\""$2"\":\""$2"\""}' | join_csv)
    [ -n "$CAP_LOGICAL_NETWORKS" ] || CAP_LOGICAL_NETWORKS=""
    CAP_SQM_CANDIDATES=$(printf '%s\n' "$CAP_INTERFACES" | tr ',' '\n' | tr -d '"' | awk '/^(eth|br-wan)/ {print}' | head -n 8 | sed 's/.*/"&"/' | join_csv)
    [ -n "$CAP_SQM_CANDIDATES" ] || CAP_SQM_CANDIDATES=""
    CAP_FIREWALL="unknown"
    command -v fw4 >/dev/null 2>&1 && CAP_FIREWALL="firewall4"
    command -v fw3 >/dev/null 2>&1 && CAP_FIREWALL="firewall3"
    CAP_SWITCH="unknown"
    [ -d /sys/class/net ] && { command -v bridge >/dev/null 2>&1 && CAP_SWITCH="dsa"; command -v swconfig >/dev/null 2>&1 && CAP_SWITCH="swconfig"; }
    CAP_PACKAGES=$(opkg list-installed 2>/dev/null | awk '{print $1}' | grep -E '^(wireguard|usteer|sqm|luci-app-sqm|kmod-sched-cake|firewall[34])' | head -n 32 | sed 's/.*/"&"/' | join_csv)
    [ -n "$CAP_PACKAGES" ] || CAP_PACKAGES=""

    # 2. RECOLECCIÓN WIRELESS AVANZADA (Parser de 3 líneas para iwinfo)
    WIFI_DATA="{"
    FIRST_IFACE=1
    # Escaneamos interfaces incluyendo phy (común en drivers ath9k/WNDR3700)
    for IFACE in $(ls /sys/class/net | grep -E "wlan|ath|radio|ra|phy"); do
        [ $FIRST_IFACE -eq 0 ] && WIFI_DATA="$WIFI_DATA,"
        
        ASSOCLIST=$(iwinfo "$IFACE" assoclist 2>/dev/null | awk '
            BEGIN { printf "[" }
            /^[0-9A-F:]+/ { 
                if (count > 0) printf "},"
                printf "{\"mac\":\"%s\",\"signal\":%d,\"noise\":%d,\"inactive\":%d", $1, $2, $5, $9
                count++
            }
            /RX:/ { 
                mcs="null"; mhz="unknown"; pkts="0"
                if ($4 == "MCS") { mcs=$5; sub(/,/, "", mcs); mhz=$6; pkts=$7 }
                else { pkts=$4 }
                printf ",\"rx_rate\":\"%s\",\"rx_mcs\":%s,\"rx_mhz\":\"%s\",\"rx_pkts\":%d", $2, mcs, mhz, pkts
            }
            /TX:/ { 
                mcs="null"; mhz="unknown"; pkts="0"
                if ($4 == "MCS") { mcs=$5; sub(/,/, "", mcs); mhz=$6; pkts=$7 }
                else { pkts=$4 }
                printf ",\"tx_rate\":\"%s\",\"tx_mcs\":%s,\"tx_mhz\":\"%s\",\"tx_pkts\":%d", $2, mcs, mhz, pkts
            }
            /expected throughput:/ {
                printf ",\"expected_throughput\":\"%s\"", $3
            }
            END { 
                if (count > 0) printf "}"
                printf "]"
            }
        ')
        
        [ "$ASSOCLIST" = "[" ] && ASSOCLIST="[]"
        WIFI_DATA="$WIFI_DATA \"$IFACE\": $ASSOCLIST"
        FIRST_IFACE=0
    done
    WIFI_DATA="$WIFI_DATA }"

    # 2.5 BANDWIDTH SENTINEL (Top Talkers)
    STATE_FILE="/tmp/bandwidth_state"
    TOP_TALKERS=$(
        for IFACE in $(ls /sys/class/net | grep -E "wlan|ath|radio|ra|phy"); do
            iw dev "$IFACE" station dump 2>/dev/null | awk '
                /^Station/ { mac=$2 }
                /rx bytes:/  { rx=$3 }  # lowercase - this is what iw outputs
                /tx bytes:/  { tx=$3; if (mac != "") print mac, rx, tx }
            '
        done | awk -v state_file="$STATE_FILE" -v ts="$(date +%s)" '
            BEGIN {
                while ((getline < state_file) > 0) {
                    if (NF >= 4) {
                        prev_rx[$1] = $2; prev_tx[$1] = $3; prev_ts[$1] = $4
                    }
                }
                close(state_file)
            }
            {
                mac = $1; rx = $2; tx = $3
                if (mac in prev_rx) {
                    dt = ts - prev_ts[mac]
                    if (dt == 0) dt = 1
                    diff_rx = rx - prev_rx[mac]
                    diff_tx = tx - prev_tx[mac]
                    if (diff_rx < 0) diff_rx = 0
                    if (diff_tx < 0) diff_tx = 0
                    rate_rx = int(diff_rx / dt)
                    rate_tx = int(diff_tx / dt)
                    total_rate = rate_rx + rate_tx
                    
                    rates[mac] = total_rate
                    details_rx[mac] = rate_rx
                    details_tx[mac] = rate_tx
                }
                new_state[mac] = rx " " tx " " ts
            }
            END {
                printf "" > state_file
                for (m in new_state) {
                    print m, new_state[m] > state_file
                }
                close(state_file)
                
                n = 0
                for (m in rates) { arr[n] = m; n++ }
                for (i=0; i<n; i++) {
                    for (j=i+1; j<n; j++) {
                        if (rates[arr[j]] > rates[arr[i]]) {
                            temp = arr[i]; arr[i] = arr[j]; arr[j] = temp
                        }
                    }
                }
                
                printf "["
                limit = (n < 5) ? n : 5
                for (i=0; i<limit; i++) {
                    m = arr[i]
                    if (i > 0) printf ","
                    printf "{\"mac\":\"%s\",\"rate_rx\":%d,\"rate_tx\":%d,\"total_rate\":%d}", m, details_rx[m], details_tx[m], rates[m]
                }
                printf "]"
            }
        '
    )
    [ -z "$TOP_TALKERS" ] && TOP_TALKERS="[]"

    IFACE_STATS=$(awk '
        BEGIN { printf "{" }
        NR > 2 {
            sub(/:/, "", $1)
            if (count > 0) printf ","
            printf "\"%s\":{\"rx_bytes\":%s,\"tx_bytes\":%s}", $1, $2, $10
            count++
        }
        END { printf "}" }
    ' /proc/net/dev)

    # 3. DESCUBRIMIENTO L2 / ECHO_LOCATION (Tabla ARP, Bridge, LLDP, Port Status)
    ARP_TABLE=$(cat /proc/net/arp | awk '
        BEGIN { printf "[" }
        NR > 1 {
            if (NR > 2) printf ","
            printf "{\"ip\":\"%s\",\"mac\":\"%s\",\"device\":\"%s\"}", $1, $4, $6
        }
        END { printf "]" }
    ')

    BRIDGE_TABLE=$(brctl showmacs br-lan 2>/dev/null | awk '
        BEGIN { printf "[" }
        NR > 1 {
            if (NR > 2) printf ","
            printf "{\"port\":\"%s\",\"mac\":\"%s\",\"is_local\":\"%s\"}", $1, $2, $3
        }
        END { printf "]" }
    ')

    BR_STATUS=$(ubus call network.device status '{"name":"br-lan"}' 2>/dev/null)
    [ -z "$BR_STATUS" ] && BR_STATUS="{}"
    
    LLDP_INFO=$(lldpctl -f json 2>/dev/null)
    [ -z "$LLDP_INFO" ] && LLDP_INFO="{}"

    NEIGHBOR_STATS="{\"arp_table\": $ARP_TABLE, \"bridge_table\": $BRIDGE_TABLE, \"br_status\": $BR_STATUS, \"lldp_info\": $LLDP_INFO}"

    # 4. DHCP LEASES
    DHCP_LEASES=$(ubus call dhcp ipv4leases 2>/dev/null || echo "{\"leases\":[]}")

    # 5. LOGS RECIENTES (Últimas 20 líneas de syslog)
    # El colector tiene un límite de 2 s: logd nunca puede impedir el POST
    # de telemetría ni marcar el nodo como offline.
    SYS_LOGS=$(collect_recent_logs)

    # 5.5 FLOW_SENSE – Top 20 destinos activos desde /proc/net/nf_conntrack (ZERO CPU overhead)
    # Lee únicamente conexiones ESTABLISHED, extrae dst_ip y dst_port, agrupa y cuenta.
    # Sin exports IPFIX, sin fprobe, sin softflowd — puro awk sobre el pseudo-filesystem del kernel.
    FLOW_SENSE_DATA="[]"
    if [ -f /proc/net/nf_conntrack ]; then
        FLOW_SENSE_DATA=$(awk '
            /ESTABLISHED/ {
                proto = ""
                src = ""; dst = ""; dport = "0"
                # Determine protocol from field 1 (tcp/udp/icmp)
                proto = $3
                # Walk fields looking for dst= and dport=
                for (i = 1; i <= NF; i++) {
                    n = split($i, kv, "=")
                    if (n == 2) {
                        if (kv[1] == "dst" && dst == "") dst = kv[2]      # first dst = src-side reply dst
                        if (kv[1] == "src" && src == "") src = kv[2]      # first src = originator
                        if (kv[1] == "dport" && dport == "0") dport = kv[2]
                    }
                }
                # Skip loopback and link-local
                if (dst ~ /^127\./ || dst ~ /^169\.254/) next
                key = proto ":" dst ":" dport
                count[key]++
                # Store first-seen src for context
                if (!(key in srcs)) srcs[key] = src
            }
            END {
                # Sort by count descending (bubble sort, max 20 top entries)
                n = 0
                for (k in count) { keys[n] = k; n++ }
                for (i = 0; i < n; i++) {
                    for (j = i+1; j < n; j++) {
                        if (count[keys[j]] > count[keys[i]]) {
                            t = keys[i]; keys[i] = keys[j]; keys[j] = t
                        }
                    }
                }
                limit = (n < 20) ? n : 20
                printf "["
                for (i = 0; i < limit; i++) {
                    k = keys[i]
                    split(k, parts, ":")
                    if (i > 0) printf ","
                    printf "{\"proto\":\"%s\",\"dst\":\"%s\",\"dport\":%s,\"conns\":%d,\"sample_src\":\"%s\"}",
                        parts[1], parts[2], parts[3], count[k], srcs[k]
                }
                printf "]"
            }
        ' /proc/net/nf_conntrack 2>/dev/null)
        [ -z "$FLOW_SENSE_DATA" ] && FLOW_SENSE_DATA="[]"
    fi

    # 6. CONSTRUCCIÓN DEL PAYLOAD
    CHANGE_SET_STATUS_PAYLOAD=$(change_set_status_json)
    TRANSACTION_STATUS_PAYLOAD=$(transaction_status_json)
    PAYLOAD=$(cat <<EOF
{
    "device_id": "$DEVICE_ID",
    "agent_version": "$AGENT_VERSION",
    "timestamp": $(date +%s),
    "board": $BOARD,
    "system": $SYS_INFO,
    "capabilities": {"device_change_set":{"version":2,"namespaces":["system","dhcp","firewall","dropbear","sqm"],"max_operations":8,"confirmation_policies":["local_auto"]},"openwrt_release":"$CAP_RELEASE","architecture":"$CAP_ARCH","kernel":"$CAP_KERNEL","ram_mb":${CAP_RAM_MB:-0},"flash_mb":${CAP_FLASH_MB:-0},"interfaces":[${CAP_INTERFACES}],"radios":[${CAP_RADIOS}],"wifi_device_sections":[${CAP_WIFI_DEVICES}],"wifi_iface_sections":[${CAP_WIFI_IFACES}],"logical_networks":{${CAP_LOGICAL_NETWORKS}},"sqm_candidates":[${CAP_SQM_CANDIDATES}],"switch_stack":"$CAP_SWITCH","firewall":"$CAP_FIREWALL","packages":[${CAP_PACKAGES}]},
    "wireless_stations": $WIFI_DATA,
    "top_talkers": $TOP_TALKERS,
    "iface_stats": $IFACE_STATS,
    "neighbor_stats": $NEIGHBOR_STATS,
    "dhcp": $DHCP_LEASES,
    "flow_sense": $FLOW_SENSE_DATA,
    "logs": "$SYS_LOGS",
    "transaction": $TRANSACTION_STATUS_PAYLOAD,
    "change_set_transaction": $CHANGE_SET_STATUS_PAYLOAD,
    "survey_id": "$SURVEY_ID",
    "neighbor_aps": $NEIGHBOR_APS
}
EOF
)

    # 6. TELEMETRY (using the device token and rollback checks)
    # The device token both routes the request to its tenant and authenticates
    # this enrolled device.
    TELEMETRY_HEADERS="-H X-Device-Token:$DEVICE_TOKEN"
    HTTP_CODE=$(controller_curl -m 5 -s -X POST \
        -H "Content-Type: application/json" \
        $TELEMETRY_HEADERS \
        -d "$PAYLOAD" \
        "$TELEMETRY_URL" -w "%{http_code}" -o /dev/null)

    if [ "$HTTP_CODE" = "202" ]; then
        T_FAILS=0
    else
        T_FAILS=$((T_FAILS+1))
        logger -t agent "Telemetry failed ($HTTP_CODE). Fail count: $T_FAILS"

        if [ $T_FAILS -ge 3 ]; then
            logger -t agent "Telemetry failed 3 times. Initiating rollback."
            if [ -f "$0.old" ]; then
                mv "$0.old" "$0"
                # Re-exec the rolled-back script in-place so procd does not count
                # this as a crash. Belt-and-suspenders: the init.d/agent script
                # uses generous respawn thresholds too.
                exec /bin/sh "$0" || exit 1
            fi
        fi
    fi

    # 7. OBTENCIÓN DE CONFIGURACIÓN E INYECCIÓN DE LLAVE SSH
    # El controlador envía la llave pública en la respuesta de configuración
    CONFIG_HEADERS="-H X-Device-Token:$DEVICE_TOKEN"
    CONFIG_RESPONSE_FILE="/tmp/nerve-agent-config.$$"
    CONFIG_HTTP_CODE=$(controller_curl -m 5 -s -X GET $CONFIG_HEADERS "$CONFIG_URL" -w "%{http_code}" -o "$CONFIG_RESPONSE_FILE")
    CONFIG_RESPONSE=$(cat "$CONFIG_RESPONSE_FILE" 2>/dev/null || true)
    rm -f "$CONFIG_RESPONSE_FILE"
    if [ "$CONFIG_HTTP_CODE" != "200" ]; then
        logger -t agent "Config pull failed ($CONFIG_HTTP_CODE); preserving pending operation state."
        sleep 10
        continue
    fi

    NEW_DEVICE_TOKEN=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.device_token' 2>/dev/null)
    if [ -n "$NEW_DEVICE_TOKEN" ] && [ "$NEW_DEVICE_TOKEN" != "$DEVICE_TOKEN" ]; then
        printf '%s\n' "$NEW_DEVICE_TOKEN" > "$DEVICE_TOKEN_FILE"
        chmod 600 "$DEVICE_TOKEN_FILE"
        DEVICE_TOKEN="$NEW_DEVICE_TOKEN"
    fi

    # Controller-originated typed operations are applied locally. This is the
    # transport-safe path for changes that may interrupt the connection.
    PENDING_OPERATION_CONFIG=""
    PENDING_OPERATION_RESULT=0
    PENDING_OPERATION=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.apply_operation' 2>/dev/null)
    PENDING_CHANGE_SET=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.apply_change_set' 2>/dev/null)
    if [ -z "$PENDING_OPERATION" ] && [ -z "$PENDING_CHANGE_SET" ] && [ -f "$NERVE_OPERATION_STATUS_FILE" ]; then
        rm -f "$NERVE_OPERATION_STATUS_FILE"
    fi
    if [ -n "$PENDING_CHANGE_SET" ] && [ -n "$PENDING_OPERATION" ]; then
        logger -t agent "Controller returned conflicting operation and changeset; refusing both."
        PENDING_OPERATION_RESULT=1
        CONFLICT_CHANGE_SET_ID=$(echo "$PENDING_CHANGE_SET" | jsonfilter -e '@.change_set_id' 2>/dev/null)
        CONFLICT_CHANGE_SET_DEVICE_ID=$(echo "$PENDING_CHANGE_SET" | jsonfilter -e '@.device_id' 2>/dev/null)
        CONFLICT_CHANGE_SET_PLAN_HASH=$(echo "$PENDING_CHANGE_SET" | jsonfilter -e '@.plan_hash' 2>/dev/null)
        CONFLICT_CHANGE_SET_GENERATION=$(echo "$PENDING_CHANGE_SET" | jsonfilter -e '@.generation' 2>/dev/null)
        if [ -n "$CONFLICT_CHANGE_SET_ID" ] && [ -n "$CONFLICT_CHANGE_SET_DEVICE_ID" ] && [ -n "$CONFLICT_CHANGE_SET_PLAN_HASH" ] && [ -n "$CONFLICT_CHANGE_SET_GENERATION" ]; then
            change_set_status_write "$CONFLICT_CHANGE_SET_ID" "$CONFLICT_CHANGE_SET_DEVICE_ID" "$CONFLICT_CHANGE_SET_PLAN_HASH" "$CONFLICT_CHANGE_SET_GENERATION" REJECTED "controller returned conflicting pending work" || true
        fi
    elif [ -n "$PENDING_CHANGE_SET" ]; then
        PENDING_OPERATION_RESULT=1
        PENDING_OPERATION_CONFIG=$(echo "$PENDING_CHANGE_SET" | jsonfilter -e '@.operations[0].config' 2>/dev/null)
        if apply_pending_change_set "$PENDING_CHANGE_SET"; then
            logger -t agent "Controller changeset processed: $PENDING_OPERATION_CONFIG"
        else
            logger -t agent "Controller changeset failed or was deferred: $PENDING_OPERATION_CONFIG"
        fi
    elif [ -n "$PENDING_OPERATION" ]; then
        PENDING_OPERATION_CONFIG=$(echo "$PENDING_OPERATION" | jsonfilter -e '@.config' 2>/dev/null)
        PENDING_OPERATION_RESULT=1
        if apply_pending_operation "$PENDING_OPERATION"; then
            logger -t agent "Controller operation processed: $PENDING_OPERATION_CONFIG"
        else
            logger -t agent "Controller operation failed or was deferred: $PENDING_OPERATION_CONFIG"
        fi
    fi
    transaction_prune_terminal

    # 7.0 WIFI_SURVEY: detect survey mode from controller. When active:
    #   - telemetry interval drops to 2s (vs 10s normal)
    #   - payload includes "survey_id" so the backend tags per-station signal
    #     samples with the active survey and writes them to InfluxDB.
    #   - "neighbor_aps" snapshot is included so the dashboard can show
    #     "what other APs this device can hear from this location".
    SURVEY_MODE=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.survey_mode' 2>/dev/null)
    SURVEY_ID=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.survey_id' 2>/dev/null)
    [ "$SURVEY_MODE" != "true" ] && SURVEY_MODE="false"
    [ -z "$SURVEY_ID" ] && SURVEY_ID=""

    if [ "$SURVEY_MODE" = "true" ]; then
        # Build a compact neighbor_aps snapshot. iwinfo scan returns a
        # human-readable table; we only need BSSID,SSID,channel,signal per row.
        NEIGHBOR_APS=$(for IFACE in $(ls /sys/class/net | grep -E "wlan|ath|radio|ra|phy"); do
            iwinfo "$IFACE" scan 2>/dev/null | awk -v iface="$IFACE" '
                /Address:/ { bssid = $2 }
                /ESSID:/   { essid = ""; for (i=2; i<=NF; i++) essid = essid (i==2?"":" ") $i; gsub(/"/, "", essid) }
                /Channel:/ { chan = $2 }
                /Signal:/  { sig = $2 " " $3
                              if (count > 0) printf ","
                              printf "{\"iface\":\"%s\",\"bssid\":\"%s\",\"ssid\":\"%s\",\"channel\":%s,\"signal\":\"%s\"}", iface, bssid, essid, chan, sig
                              count++
                              bssid=""; essid=""; chan=""; sig=""
                            }
                END { exit }
            '
        done | awk 'BEGIN{first=1} { if(NR>0){ if(!first)printf ","; printf "%s",$0; first=0} }'
        )
        [ -z "$NEIGHBOR_APS" ] && NEIGHBOR_APS=""
        # Cap neighbor_aps to 64 entries to keep payload small (and prevent
        # a busy AP environment from ballooning telemetry).
        if [ -n "$NEIGHBOR_APS" ]; then
            NEIGHBOR_APS=$(printf '%s\n' "$NEIGHBOR_APS" | awk -F '},' 'NR <= 64 { if (NR > 1) printf ","; printf "%s", $0 }')
            NEIGHBOR_APS="[$NEIGHBOR_APS]"
        else
            NEIGHBOR_APS="[]"
        fi
    else
        NEIGHBOR_APS="[]"
    fi

    if [ -n "$CONFIG_RESPONSE" ]; then
        # Extraer llave pública usando jsonfilter (nativo en OpenWrt)
        PUBKEY=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.ssh_pubkey' 2>/dev/null)
        
        if [ -n "$PUBKEY" ]; then
            # Crear archivo si no existe y asegurar permisos
            touch /etc/dropbear/authorized_keys
            chmod 600 /etc/dropbear/authorized_keys
            
            # Inyectar solo si no está ya presente
            grep -q "$PUBKEY" /etc/dropbear/authorized_keys || {
                echo "$PUBKEY" >> /etc/dropbear/authorized_keys
                logger -t agent "SSH_KEY_INJECTED: Master controller key added."
            }
        fi

        # 7.5 CONFIGURACIÓN WIRELESS CENTRALIZADA
        NEW_WIFI_HASH=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireless' 2>/dev/null | sha256sum | awk '{print $1}')
        OLD_WIFI_HASH=$(cat "$NERVE_WIFI_HASH_FILE" 2>/dev/null)
        
        if [ "$PENDING_OPERATION_RESULT" -eq 0 ] && [ -n "$NEW_WIFI_HASH" ] && [ "$NEW_WIFI_HASH" != "$OLD_WIFI_HASH" ]; then
            logger -t agent "DEBUG: NEW=$NEW_WIFI_HASH OLD=$OLD_WIFI_HASH URL=$CONFIG_URL RES_LEN=${#CONFIG_RESPONSE}"
            logger -t agent "WLAN config changed. Re-provisioning radios..."
            
            WLAN_JSON=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireless.wlans' 2>/dev/null)
            case "$WLAN_JSON" in
                \[*\]) ;;
                *)
                    logger -t agent "WLAN config is invalid; refusing to mutate wireless."
                    continue
                    ;;
            esac
            WLAN_COUNT=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireless.wlans[@]' 2>/dev/null | wc -l 2>/dev/null || echo 0)
            
            if [ "$WLAN_COUNT" -ge 0 ]; then
                transaction_begin wireless "$NEW_WIFI_HASH"
                TRANSACTION_STATUS=$?
                if [ "$TRANSACTION_STATUS" -eq 10 ]; then
                    transaction_write_atomic "$NERVE_WIFI_HASH_FILE" "$NEW_WIFI_HASH"
                    logger -t agent "WLAN transaction already committed; skipping replay."
                elif [ "$TRANSACTION_STATUS" -ne 0 ]; then
                    logger -t agent "WLAN transaction is busy or unsafe; skipping apply."
                else
                (
                set -e
                while uci -q delete wireless.@wifi-iface[0]; do :; done
                for CFG in $(uci show wireless | grep -o 'wireless\.cfg_radio[0-9]_[0-9]*' | cut -d. -f2 | sort -u); do uci delete wireless.$CFG; done
                
                for RADIO in $(uci -q show wireless | grep "=wifi-device" | cut -d'.' -f2 | cut -d'=' -f1); do
                    M_BAND=""
                    R_BAND=$(ubus call network.wireless status | jsonfilter -e "@.$RADIO.config.band" 2>/dev/null)
                    if [ -n "$R_BAND" ]; then
                        if [ "$R_BAND" = "2g" ] || [ "$R_BAND" = "2g-5g" ]; then M_BAND="2.4GHz"
                        elif [ "$R_BAND" = "5g" ]; then M_BAND="5GHz"
                        fi
                    fi
                    
                    if [ -z "$M_BAND" ]; then
                        R_HW=$(ubus call network.wireless status | jsonfilter -e "@.$RADIO.config.hwmode" 2>/dev/null)
                        if [ "$R_HW" = "11a" ] || [ "$R_HW" = "11ac" ] || [ "$R_HW" = "11ax" ]; then M_BAND="5GHz"
                        elif [ "$R_HW" = "11g" ] || [ "$R_HW" = "11b" ] || [ "$R_HW" = "11n" ]; then M_BAND="2.4GHz"
                        fi
                    fi
                    
                    if [ -z "$M_BAND" ]; then
                        R_CHAN=$(ubus call network.wireless status | jsonfilter -e "@.$RADIO.config.channel" 2>/dev/null)
                        if [ "$R_CHAN" != "auto" ] && [ "$R_CHAN" -gt 14 ]; then M_BAND="5GHz"
                        else M_BAND="2.4GHz"
                        fi
                    fi

                    i=0
                    while [ $i -lt "$WLAN_COUNT" ]; do
                        W_SSID=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].ssid" 2>/dev/null)
                        W_SEC=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].security" 2>/dev/null)
                        W_KEY=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].key" 2>/dev/null)
                        W_BAND=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].band" 2>/dev/null)
                        W_ROAMING=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].ieee80211r" 2>/dev/null)
                        W_80211K=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].ieee80211k" 2>/dev/null)
                        W_80211V=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].ieee80211v" 2>/dev/null)
                        W_MFP=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].ieee80211w" 2>/dev/null)

                        W_AUTH_SERVER=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].auth_server" 2>/dev/null)

                        W_AUTH_SECRET=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].auth_secret" 2>/dev/null)

                        W_DYN_VLAN=$(echo "$CONFIG_RESPONSE" | jsonfilter -e "@.config.wireless.wlans[$i].dynamic_vlan" 2>/dev/null)
                        
                        if [ "$W_BAND" = "both" ] || [ "$W_BAND" = "$M_BAND" ]; then
                            SECTION="cfg_${RADIO}_${i}"
                            uci set wireless.$SECTION=wifi-iface
                            uci set wireless.$SECTION.device="$RADIO"
                            uci set wireless.$SECTION.network='lan'
                            uci set wireless.$SECTION.mode='ap'
                            uci set wireless.$SECTION.ssid="$W_SSID"
                            uci set wireless.$SECTION.encryption="$W_SEC"
                            [ -n "$W_KEY" ] && uci set wireless.$SECTION.key="$W_KEY"
                            

                            [ -n "$W_MFP" ] && uci set wireless.$SECTION.ieee80211w="$W_MFP"

                            if [ -n "$W_AUTH_SERVER" ] && [ "$W_AUTH_SERVER" != "null" ]; then
                                if [ "$W_AUTH_SERVER" = "AUTO" ]; then W_AUTH_SERVER="$CONTROLLER_HOST"; fi

                                uci set wireless.$SECTION.auth_server="$W_AUTH_SERVER"

                                uci set wireless.$SECTION.auth_secret="$W_AUTH_SECRET"

                            fi

                            if [ "$W_DYN_VLAN" = "1" ] || [ "$W_DYN_VLAN" = "2" ]; then

                                uci set wireless.$SECTION.dynamic_vlan="$W_DYN_VLAN"

                                uci set wireless.$SECTION.vlan_naming="1"

                                uci set wireless.$SECTION.vlan_bridge="br-vlan"

                            fi
                            
                            if [ "$W_ROAMING" = "1" ] || [ "$W_ROAMING" = "true" ]; then
                                uci set wireless.$SECTION.ieee80211r='1'
                                uci set wireless.$SECTION.ft_over_ds='0'
                                uci set wireless.$SECTION.ft_psk_generate_local='1'
                                uci set wireless.$SECTION.mobility_domain='1234'
                            fi
                            if [ "$W_80211K" = "1" ] || [ "$W_80211K" = "true" ]; then
                                uci set wireless.$SECTION.ieee80211k='1'
                            fi
                            if [ "$W_80211V" = "1" ] || [ "$W_80211V" = "true" ]; then
                                uci set wireless.$SECTION.bss_transition='1'
                                uci set wireless.$SECTION.wnm_sleep_mode='1'
                                uci set wireless.$SECTION.time_advertisement='2'
                                uci set wireless.$SECTION.time_zone='<-05>5'
                            fi
                        fi
                        i=$((i+1))
                    done
                done
                )
                APPLY_STATUS=$?
                if [ "$APPLY_STATUS" -eq 0 ]; then
                    if uci commit wireless; then
                        if transaction_mark_pending wireless "$NEW_WIFI_HASH" && wifi reload && uci show wireless >/dev/null 2>&1; then
                            if transaction_commit wireless "$NEW_WIFI_HASH" "$NERVE_WIFI_HASH_FILE" "$NEW_WIFI_HASH"; then
                                logger -t agent "WLAN config applied successfully."
                            else
                                logger -t agent "WLAN transaction commit failed; restoring snapshot."
                                transaction_recover_pending || true
                            fi
                        else
                            logger -t agent "WLAN validation/reload failed; restoring snapshot."
                            transaction_recover_pending || true
                        fi
                    else
                        logger -t agent "WLAN UCI commit failed; restoring snapshot."
                        transaction_recover_pending || true
                    fi
                else
                    logger -t agent "WLAN UCI mutation failed; restoring snapshot."
                    transaction_recover_pending || true
                fi
                fi
            else
                logger -t agent "WLAN config empty. Skipping."
            fi
        fi

        # 8. CONFIGURACIÓN DE WIREGUARD (SECURE_TUNNEL)
        WG_ENABLED=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.enabled' 2>/dev/null)
        WG_PAYLOAD=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard' 2>/dev/null)
        WG_HASH=$(printf '%s' "$WG_PAYLOAD" | sha256sum | awk '{print $1}')
        WG_APPLIED_HASH=$(cat "$NERVE_WG_HASH_FILE" 2>/dev/null)
        WG_EXISTS=$(uci -q get network.wg_nerve.proto)
        if [ "$PENDING_OPERATION_RESULT" -eq 0 ] && [ -n "$WG_PAYLOAD" ] && { [ "$WG_ENABLED" = "true" ] && { [ "$WG_EXISTS" != "wireguard" ] || [ "$WG_HASH" != "$WG_APPLIED_HASH" ]; } || [ "$WG_ENABLED" != "true" ] && [ "$WG_EXISTS" = "wireguard" ]; }; then
            logger -t agent "WIREGUARD: applying durable network transaction."
            transaction_begin network "$WG_HASH"
            TRANSACTION_STATUS=$?
            if [ "$TRANSACTION_STATUS" -eq 10 ]; then
                transaction_write_atomic "$NERVE_WG_HASH_FILE" "$WG_HASH"
                logger -t agent "WIREGUARD: transaction already committed; skipping replay."
            elif [ "$TRANSACTION_STATUS" -ne 0 ]; then
                logger -t agent "WIREGUARD: transaction is busy or unsafe; skipping apply."
            else
                (
                    set -e
                    if [ "$WG_ENABLED" = "true" ]; then
                        WG_PRIV=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.private_key' 2>/dev/null)
                        WG_PUB=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.controller_pubkey' 2>/dev/null)
                        WG_EP=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.endpoint_ip' 2>/dev/null)
                        WG_IP=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.internal_ip' 2>/dev/null)
                        WG_ALLOWED=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.wireguard.allowed_ips' 2>/dev/null)

                        uci set network.wg_nerve=interface
                        uci set network.wg_nerve.proto='wireguard'
                        uci set network.wg_nerve.private_key="$WG_PRIV"
                        uci -q delete network.wg_nerve.addresses || true
                        uci add_list network.wg_nerve.addresses="${WG_IP}/24"
                        uci set network.wg_nerve_peer=wireguard_wg_nerve
                        uci set network.wg_nerve_peer.public_key="$WG_PUB"
                        uci set network.wg_nerve_peer.endpoint_host="${WG_EP%%:*}"
                        uci set network.wg_nerve_peer.endpoint_port="${WG_EP##*:}"
                        uci set network.wg_nerve_peer.route_allowed_ips='1'
                        uci set network.wg_nerve_peer.persistent_keepalive='25'
                        uci -q delete network.wg_nerve_peer.allowed_ips || true
                        uci add_list network.wg_nerve_peer.allowed_ips="$WG_ALLOWED"
                        uci commit network
                        ifup wg_nerve
                    else
                        ifdown wg_nerve 2>/dev/null || true
                        uci -q delete network.wg_nerve || true
                        uci -q delete network.wg_nerve_peer || true
                        uci commit network
                    fi
                )
                APPLY_STATUS=$?
                if [ "$APPLY_STATUS" -eq 0 ]; then
                    if transaction_mark_pending network "$WG_HASH" && uci show network >/dev/null 2>&1; then
                        if transaction_commit network "$WG_HASH" "$NERVE_WG_HASH_FILE" "$WG_HASH"; then
                            logger -t agent "WIREGUARD: durable network transaction committed."
                        else
                            logger -t agent "WIREGUARD: transaction commit failed; restoring snapshot."
                            transaction_recover_pending || true
                        fi
                    else
                        logger -t agent "WIREGUARD: validation failed; restoring snapshot."
                        transaction_recover_pending || true
                    fi
                else
                    logger -t agent "WIREGUARD: UCI mutation failed; restoring snapshot."
                    transaction_recover_pending || true
                fi
            fi
        fi
    fi

    # 8.5 TAILSCALE / HEADSCALE ZERO TRUST OVERLAY

    if [ -n "$CONFIG_RESPONSE" ]; then

        TS_ENABLED=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.tailscale.enabled' 2>/dev/null)

        TS_KEY=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.tailscale.auth_key' 2>/dev/null)

        if [ "$TS_ENABLED" = "true" ] && [ -n "$TS_KEY" ]; then

            TS_STATUS=$(/etc/init.d/tailscale status 2>/dev/null)

            if ! echo "$TS_STATUS" | grep -q "running"; then

                logger -t agent "TAILSCALE: Starting and authenticating Zero Trust Overlay..."

                /etc/init.d/tailscale enable

                /etc/init.d/tailscale start

                sleep 2

                tailscale up --authkey "$TS_KEY" --accept-routes --reset

            fi

        elif [ "$TS_ENABLED" = "false" ]; then

            TS_STATUS=$(/etc/init.d/tailscale status 2>/dev/null)

            if echo "$TS_STATUS" | grep -q "running"; then

                logger -t agent "TAILSCALE: Disabling overlay network..."

                tailscale logout 2>/dev/null

                /etc/init.d/tailscale stop

                /etc/init.d/tailscale disable

            fi

        fi

    fi

    # 9. [THREAT_SHIELD] — nftables IP reputation enforcement
    if [ -n "$CONFIG_RESPONSE" ]; then
        TS_ENABLED=$(echo "$CONFIG_RESPONSE" | jsonfilter -e '@.config.threat_shield' 2>/dev/null)
    fi

    if [ "$TS_ENABLED" = "true" ]; then
        TS_LIST_FILE="/tmp/ts_raw.txt"
        TS_NFT_FILE="/tmp/ts.nft"
        TS_STAMP_FILE="/tmp/ts.stamp"

        # Refresh blocklist if older than 6 hours (21600s) or missing
        TS_REFRESH=0
        if [ ! -f "$TS_STAMP_FILE" ]; then
            TS_REFRESH=1
        else
            TS_STAMP=$(cat "$TS_STAMP_FILE" 2>/dev/null || echo 0)
            TS_NOW=$(date +%s)
            TS_AGE=$((TS_NOW - TS_STAMP))
            [ "$TS_AGE" -gt 21600 ] && TS_REFRESH=1
        fi

        if [ "$TS_REFRESH" = "1" ]; then
            logger -t threat_shield "Downloading reputation blocklist..."
            if controller_curl -m 60 -s -H "X-Device-Token: $DEVICE_TOKEN" \
                    "$BASE_URL/threat-shield/list" \
                    -o "$TS_LIST_FILE.tmp" 2>/dev/null; then
                TS_COUNT=$(wc -l < "$TS_LIST_FILE.tmp" 2>/dev/null || echo 0)
			if [ "$TS_COUNT" -gt 10 ]; then
				# Never install malformed, non-routable, or private ranges from a feed.
				TS_SAFE_FILE="$TS_LIST_FILE.safe"
				awk '
					function ipnum(a,b,c,d) { return (((a*256+b)*256+c)*256+d) }
					NF && !/^#/ {
						t=$1; gsub(/[;, \t].*/, "", t)
						split(t, ip, "/"); split(ip[1], o, ".")
						if (length(o) != 4 || ip[2] == "0" || ip[2] == "") next
						base=ipnum(o[1],o[2],o[3],o[4])
						if (base < ipnum(1,0,0,0) || base >= ipnum(224,0,0,0)) next
						if (base >= ipnum(10,0,0,0) && base < ipnum(11,0,0,0)) next
						if (base >= ipnum(100,64,0,0) && base < ipnum(100,128,0,0)) next
						if (base >= ipnum(127,0,0,0) && base < ipnum(128,0,0,0)) next
						if (base >= ipnum(169,254,0,0) && base < ipnum(169,255,0,0)) next
						if (base >= ipnum(172,16,0,0) && base < ipnum(172,32,0,0)) next
						if (base >= ipnum(192,168,0,0) && base < ipnum(192,169,0,0)) next
						print t
					}
				' "$TS_LIST_FILE.tmp" > "$TS_SAFE_FILE"
				TS_SAFE_COUNT=$(wc -l < "$TS_SAFE_FILE" 2>/dev/null || echo 0)
				# Require a substantial safe subset and reject the feed if any
				# line was discarded. This prevents partial/ambiguous feeds from
				# silently changing firewall policy.
				if [ "$TS_SAFE_COUNT" -lt 10 ] || [ "$TS_SAFE_COUNT" -ne "$TS_COUNT" ]; then
					rm -f "$TS_LIST_FILE.tmp" "$TS_SAFE_FILE"
					logger -t threat_shield "Rejected blocklist: insufficient safe entries"
					continue
				fi
				mv "$TS_SAFE_FILE" "$TS_LIST_FILE.tmp"

				mv "$TS_LIST_FILE.tmp" "$TS_LIST_FILE"
                    date +%s > "$TS_STAMP_FILE"
                    logger -t threat_shield "Blocklist updated: $TS_COUNT entries"

                    # Ensure table, set, and chains exist (idempotent)
                    nft list table inet threat_shield > /dev/null 2>&1 || \
                        nft add table inet threat_shield
                    nft list set inet threat_shield denylist > /dev/null 2>&1 || \
                        nft add set inet threat_shield denylist \
                            '{ type ipv4_addr; flags interval; auto-merge; }'
                    nft list chain inet threat_shield forward > /dev/null 2>&1 || {
                        nft add chain inet threat_shield forward \
                            '{ type filter hook forward priority -1; }'
                        nft add rule inet threat_shield forward \
                            ip daddr @denylist counter drop
                    }
                    nft list chain inet threat_shield input > /dev/null 2>&1 || {
                        nft add chain inet threat_shield input \
                            '{ type filter hook input priority -1; }'
                        nft add rule inet threat_shield input \
                            ip saddr @denylist counter drop
                    }

                    # Build atomic nft reload script from the list
                    awk '
                        BEGIN { print "flush set inet threat_shield denylist" }
                        NF && !/^#/ {
                            gsub(/[;, \t].*/, "")
                            if ($1 ~ /^[0-9]/) printf "add element inet threat_shield denylist { %s }\n", $1
                        }
                    ' "$TS_LIST_FILE" > "$TS_NFT_FILE"

                    # Apply atomically
                    nft -f "$TS_NFT_FILE" 2>/dev/null && \
                        logger -t threat_shield "Denylist applied to nftables"
                else
                    rm -f "$TS_LIST_FILE.tmp"
                    logger -t threat_shield "Blocklist download empty or too small, skipping"
                fi
            else
                logger -t threat_shield "Blocklist download failed"
            fi
        fi

        # Collect drop counters from nftables
        TS_DROPS_FWD=$(nft list chain inet threat_shield forward 2>/dev/null | \
            awk '/counter/{for(i=1;i<=NF;i++) if($i=="packets") print $(i+1)}' | head -1)
        TS_DROPS_IN=$(nft list chain inet threat_shield input 2>/dev/null | \
            awk '/counter/{for(i=1;i<=NF;i++) if($i=="packets") print $(i+1)}' | head -1)
        TS_DROPS_TOTAL=$(( ${TS_DROPS_FWD:-0} + ${TS_DROPS_IN:-0} ))
        TS_LOADED=$(wc -l < "$TS_LIST_FILE" 2>/dev/null || echo 0)

    else
        # Disable: remove threat_shield table if it exists
        nft list table inet threat_shield > /dev/null 2>&1 && \
            nft delete table inet threat_shield 2>/dev/null || true
        TS_DROPS_TOTAL=0
        TS_LOADED=0
        rm -f /tmp/ts.stamp 2>/dev/null
    fi

    # WIFI_SURVEY: tighten the loop to 2s when an active survey exists.
    # The controller toggles survey_mode via /api/devices/{id}/config; the
    # next loop iteration picks up the new interval.
    if [ "$SURVEY_MODE" = "true" ]; then
        sleep 2
    else
        sleep 10
    fi
done
