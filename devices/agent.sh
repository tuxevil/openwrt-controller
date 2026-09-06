#!/bin/sh

# === CONFIGURACIÓN ===
# The signed artifact is immutable. Runtime credentials and the controller
# endpoint live in a root-owned file written by the image bootstrap.
NERVE_CONFIG_FILE="${NERVE_CONFIG_FILE:-/etc/nerve/agent.conf}"
if [ -r "$NERVE_CONFIG_FILE" ]; then
    . "$NERVE_CONFIG_FILE"
fi
CONTROLLER_URL="${CONTROLLER_URL:-}"
CONTROLLER_IP="${CONTROLLER_IP:-}"
PORT="${PORT:-3000}"
if [ -z "$CONTROLLER_URL" ] && [ -n "$CONTROLLER_IP" ]; then
    CONTROLLER_URL="http://$CONTROLLER_IP:$PORT/api"
fi
BASE_URL="$CONTROLLER_URL"
TELEMETRY_URL="$BASE_URL/telemetry"
DEVICE_ID_FILE="${DEVICE_ID_FILE:-/etc/nerve-device-id}"
DEVICE_ID="$(cat "$DEVICE_ID_FILE" 2>/dev/null || true)"
if [ -z "$DEVICE_ID" ]; then
    # Seed identity once. A bridge MAC can change when a NIC is added later.
    DEVICE_ID=$(cat /sys/class/net/br-lan/address 2>/dev/null | tr '[:lower:]' '[:upper:]' || cat /sys/class/net/eth0/address 2>/dev/null | tr '[:lower:]' '[:upper:]')
    if [ -n "$DEVICE_ID" ]; then
        printf '%s\n' "$DEVICE_ID" > "$DEVICE_ID_FILE"
        chmod 600 "$DEVICE_ID_FILE"
    fi
fi
CONFIG_URL="$BASE_URL/devices/$DEVICE_ID/config"
DEVICE_TOKEN_FILE="${DEVICE_TOKEN_FILE:-/etc/nerve-device-token}"
DEVICE_TOKEN="$(cat "$DEVICE_TOKEN_FILE" 2>/dev/null || true)"
ENROLLMENT_TOKEN_FILE="${ENROLLMENT_TOKEN_FILE:-/etc/nerve/enrollment-token}"
ENROLLMENT_NONCE_FILE="${ENROLLMENT_NONCE_FILE:-/etc/nerve/enrollment-nonce}"
AGENT_UPDATE_PUBLIC_KEY_FILE="${AGENT_UPDATE_PUBLIC_KEY_FILE:-/etc/nerve/agent-update-public-key}"
AGENT_UPDATE_PUBLIC_KEY="$(cat "$AGENT_UPDATE_PUBLIC_KEY_FILE" 2>/dev/null || true)"
NERVE_TRANSACTION_ROOT="${NERVE_TRANSACTION_ROOT:-/etc/nerve/transactions}"
NERVE_CONFIG_ROOT="${NERVE_CONFIG_ROOT:-/etc/config}"
NERVE_WIFI_HASH_FILE="${NERVE_WIFI_HASH_FILE:-$NERVE_TRANSACTION_ROOT/wifi_config.hash}"
NERVE_WG_HASH_FILE="${NERVE_WG_HASH_FILE:-$NERVE_TRANSACTION_ROOT/wg_config.hash}"
NERVE_OPERATION_STATUS_FILE="${NERVE_OPERATION_STATUS_FILE:-$NERVE_TRANSACTION_ROOT/operation_status}"

transaction_valid_id() {
    case "$1" in
        ''|*[!A-Za-z0-9._-]*) return 1 ;;
    esac
    [ "$1" != "." ] && [ "$1" != ".." ] || return 1
    [ "${#1}" -le 128 ]
}

transaction_valid_config() {
    case "$1" in
        wireless|network|dhcp|firewall|dropbear|system|sqm) return 0 ;;
    esac
    return 1
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
    if ! printf '%s' "$public_key_base64" | base64 -d > "$verify_dir/public.raw" 2>/dev/null; then
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
    if ! printf '%s' "$signature_base64" | base64 -d > "$verify_dir/signature" 2>/dev/null; then
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

transaction_recover_one() {
    local transaction_id="$1"
    local transaction_path transaction_state transaction_config transaction_tmp
    if ! transaction_valid_id "$transaction_id"; then
        logger -t agent "TRANSACTION_RECOVERY_FAILED: invalid active operation id"
        return 1
    fi

    transaction_path=$(transaction_dir "$transaction_id")
    transaction_state=$(cat "$transaction_path/state" 2>/dev/null || true)
    case "$transaction_state" in
        COMMITTED|RESTORED)
            if [ "$(cat "$NERVE_TRANSACTION_ROOT/active" 2>/dev/null || true)" = "$transaction_id" ]; then
                rm -f "$NERVE_TRANSACTION_ROOT/active"
            fi
            return 0
            ;;
        APPLYING|PENDING_CONFIRM|ROLLING_BACK)
            ;;
        *)
            logger -t agent "TRANSACTION_RECOVERY_FAILED: unknown state $transaction_state"
            return 1
            ;;
    esac

    transaction_config=$(cat "$transaction_path/config" 2>/dev/null || true)
    if ! transaction_valid_config "$transaction_config"; then
        logger -t agent "TRANSACTION_RECOVERY_FAILED: invalid config namespace"
        return 1
    fi

    transaction_write_atomic "$transaction_path/state" ROLLING_BACK || return 1
    if [ "$(cat "$transaction_path/backup_exists" 2>/dev/null || true)" = "1" ]; then
        transaction_tmp="$NERVE_CONFIG_ROOT/$transaction_config.$$"
        cp "$transaction_path/backup" "$transaction_tmp" || return 1
        mv "$transaction_tmp" "$NERVE_CONFIG_ROOT/$transaction_config" || return 1
    else
        rm -f "$NERVE_CONFIG_ROOT/$transaction_config"
    fi
    if command -v uci >/dev/null 2>&1; then
        uci revert "$transaction_config" 2>/dev/null || true
        uci commit "$transaction_config" || return 1
    fi
    transaction_restart_config "$transaction_config" || return 1
    transaction_write_atomic "$transaction_path/state" RESTORED || return 1
    if [ "$(cat "$NERVE_TRANSACTION_ROOT/active" 2>/dev/null || true)" = "$transaction_id" ]; then
        rm -f "$NERVE_TRANSACTION_ROOT/active"
    fi
    rm -f "$transaction_path/backup" "$transaction_path/backup_exists"
    transaction_write_atomic "$NERVE_TRANSACTION_ROOT/last" "$transaction_id" || return 1
    logger -t agent "TRANSACTION_RECOVERED: restored $transaction_config for $transaction_id"
}

transaction_recover_pending() {
    local transaction_id transaction_path transaction_state
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
        esac
    done
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
    local active_id active_state transaction_path transaction_state transaction_tmp
    transaction_valid_config "$transaction_config" || return 1
    transaction_valid_id "$transaction_id" || return 1
    mkdir -p "$NERVE_TRANSACTION_ROOT" || return 1
    chmod 700 "$NERVE_TRANSACTION_ROOT" 2>/dev/null || return 1

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
                return 10
                ;;
            APPLYING|PENDING_CONFIRM|ROLLING_BACK)
                return 1
                ;;
            RESTORED)
                rm -rf "$transaction_path"
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
    local transaction_id transaction_path transaction_config transaction_state
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
    transaction_config=$(cat "$transaction_path/config" 2>/dev/null || true)
    transaction_state=$(cat "$transaction_path/state" 2>/dev/null || true)
    if ! transaction_valid_config "$transaction_config"; then
        printf '{}'
        return 0
    fi
    printf '{"id":"%s","config":"%s","state":"%s"}' "$transaction_id" "$transaction_config" "$transaction_state"
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
    local operation_id operation_config operation_hash operation_count operation_index
    local operation_action operation_command_config operation_section operation_option operation_value
    local transaction_status operation_status
    operation_id=$(printf '%s' "$operation_json" | jsonfilter -e '@.operation_id' 2>/dev/null)
    operation_config=$(printf '%s' "$operation_json" | jsonfilter -e '@.config' 2>/dev/null)
    operation_hash=$(printf '%s' "$operation_json" | jsonfilter -e '@.plan_hash' 2>/dev/null)
    operation_count=$(printf '%s' "$operation_json" | jsonfilter -e '@.commands[@]' 2>/dev/null | wc -l 2>/dev/null || echo 0)
    transaction_valid_id "$operation_id" || return 1
    transaction_valid_id "$operation_hash" || return 1
    [ "$operation_hash" = "$operation_id" ] || return 1
    transaction_valid_config "$operation_config" || return 1
    [ "$operation_count" -gt 0 ] || return 1

    transaction_begin "$operation_config" "$operation_id"
    transaction_status=$?
    if [ "$transaction_status" -eq 10 ]; then
        return 0
    fi
    [ "$transaction_status" -eq 0 ] || return 1

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
        transaction_write_atomic "$NERVE_OPERATION_STATUS_FILE" "$operation_id" || true
        return 1
    fi

    transaction_mark_pending "$operation_config" "$operation_id" || {
        transaction_recover_pending || true
        return 1
    }
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
    transaction_commit "$operation_config" "$operation_id" "$NERVE_TRANSACTION_ROOT/operation_${operation_config}.hash" "$operation_hash" || {
        transaction_recover_pending || true
        transaction_write_atomic "$NERVE_OPERATION_STATUS_FILE" "$operation_id" || true
        return 1
    }
    transaction_write_atomic "$NERVE_OPERATION_STATUS_FILE" "$operation_id" || return 1
    logger -t agent "TRANSACTION_COMMITTED: $operation_config operation $operation_id"
    return 0
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
        SELF_TEST_OPERATION='{"operation_id":"self-operation","plan_hash":"self-operation","config":"system","commands":[{"action":"set","config":"system","section":"@system[0]","option":"hostname","value":"lab-router"}],"auto_confirm":true}'
    fi
    apply_pending_operation "$SELF_TEST_OPERATION" || exit 1
    exit 0
fi

if [ "${1:-}" = "--self-test-status" ]; then
    transaction_status_json
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

if [ -z "$BASE_URL" ] || [ -z "$DEVICE_ID" ]; then
    logger -t agent "Runtime configuration or device identity is missing"
    exit 1
fi

# Recover before the first network request. A transaction left in APPLYING or
# PENDING_CONFIRM is never trusted after a process crash or reboot.
if ! transaction_recover_pending; then
    logger -t agent "Persistent transaction recovery failed; refusing to start"
    exit 1
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
    enrollment_payload=$(printf '{"device_id":"%s","nonce":"%s","capabilities":{"architecture":"%s","kernel":"%s"}}' \
        "$DEVICE_ID" "$enrollment_nonce" "$enrollment_arch" "$enrollment_kernel")
    enrollment_response_file="/tmp/nerve-enrollment-response.$$"
    enrollment_http_code=$(curl -m 10 -sS -X POST \
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

# logd is a local dependency, not part of the telemetry heartbeat.  On some
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

# Local/CI seam for the bounded log collector.  It does not start the agent
# loop or touch device configuration.
if [ "${1:-}" = "--self-test-log-collection" ]; then
    collect_recent_logs
    exit 0
fi

# Instalar dependencias si faltan (opcional)
if ! command -v tcpdump >/dev/null 2>&1; then
    logger -t agent "Installing missing tcpdump..."
    if command -v apk >/dev/null 2>&1; then
        apk update
        apk add tcpdump iperf3 sqm-scripts kmod-sched-cake tailscale
        apk search -e libndpi | grep -q libndpi && apk add libndpi || true
        if apk info -e wpad-basic-wolfssl >/dev/null 2>&1 || apk info -e wpad-basic-mbedtls >/dev/null 2>&1; then
            apk del wpad-basic-wolfssl wpad-basic-mbedtls 2>/dev/null || true
            apk add wpad-mesh-wolfssl || true
        fi
    elif command -v opkg >/dev/null 2>&1; then
        opkg update
        if opkg list-installed | grep -q "wpad-basic"; then opkg remove wpad-basic-wolfssl wpad-basic-mbedtls; opkg install wpad-mesh-wolfssl; fi
        opkg install tcpdump iperf3 sqm-scripts kmod-sched-cake tailscale
        opkg list libndpi | grep -q libndpi && opkg install libndpi || true
    fi
fi
# apk update && apk add iwinfo curl

if ! command -v curl >/dev/null 2>&1 || ! command -v jsonfilter >/dev/null 2>&1; then
    logger -t agent "Bootstrap prerequisites are unavailable"
    exit 1
fi
if ! bootstrap_agent; then
    exit 1
fi

T_FAILS=0

while true; do
    # 0. CHECK AUTO-UPDATE
    # The signed artifact is hashed byte-for-byte because runtime settings are
    # stored outside this file.
    AGENT_VERSION=$(sha256sum "$0" | awk '{print $1}')
    LATEST_JSON=$(curl -m 5 -s -X GET -H "X-Device-Token: $DEVICE_TOKEN" "$BASE_URL/agent/latest")
    
    if [ -n "$LATEST_JSON" ]; then
        LATEST_HASH=$(echo "$LATEST_JSON" | jsonfilter -e '@.version_hash' 2>/dev/null)
        if [ -n "$LATEST_HASH" ] && [ "$LATEST_HASH" != "$AGENT_VERSION" ]; then
            LATEST_SIGNATURE=$(echo "$LATEST_JSON" | jsonfilter -e '@.signature' 2>/dev/null)
            LATEST_SIGNATURE_ALGORITHM=$(echo "$LATEST_JSON" | jsonfilter -e '@.signature_algorithm' 2>/dev/null)
            logger -t agent "New agent version found: $LATEST_HASH. Downloading..."
            if curl -m 10 -s -X GET -H "X-Device-Token: $DEVICE_TOKEN" "$BASE_URL/agent/latest/raw" -o "$0.tmp"; then
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
                    if chmod +x "$0.tmp" && cp "$0" "$0.old" && mv "$0.tmp" "$0"; then
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
    CAP_INTERFACES=$(ip -o link show 2>/dev/null | awk -F': ' '{print $2}' | cut -d'@' -f1 | head -n 32 | sed 's/.*/"&"/' | paste -sd, -)
    [ -n "$CAP_INTERFACES" ] || CAP_INTERFACES=""
    CAP_RADIOS=$(iwinfo 2>/dev/null | awk '/^[a-zA-Z0-9_.-]+[[:space:]]+ESSID:/ {print $1}' | head -n 16 | sed 's/.*/"&"/' | paste -sd, -)
    [ -n "$CAP_RADIOS" ] || CAP_RADIOS=""
    CAP_WIFI_DEVICES=$(uci -q show wireless 2>/dev/null | awk -F'[.=]' '/=wifi-device/ {print $2}' | head -n 8 | sed 's/.*/"&"/' | paste -sd, -)
    CAP_WIFI_IFACES=$(uci -q show wireless 2>/dev/null | awk -F'[.=]' '/=wifi-iface/ {print $2}' | head -n 16 | sed 's/.*/"&"/' | paste -sd, -)
    [ -n "$CAP_WIFI_DEVICES" ] || CAP_WIFI_DEVICES=""
    [ -n "$CAP_WIFI_IFACES" ] || CAP_WIFI_IFACES=""
    CAP_LOGICAL_NETWORKS=$(uci -q show network 2>/dev/null | awk -F'[.=]' '/\.device=/ {print $1":"$2}' | head -n 16 | awk -F: '{print "\""$2"\":\""$2"\""}' | paste -sd, -)
    [ -n "$CAP_LOGICAL_NETWORKS" ] || CAP_LOGICAL_NETWORKS=""
    CAP_SQM_CANDIDATES=$(printf '%s\n' "$CAP_INTERFACES" | tr ',' '\n' | tr -d '"' | awk '/^(eth|br-wan)/ {print}' | head -n 8 | sed 's/.*/"&"/' | paste -sd, -)
    [ -n "$CAP_SQM_CANDIDATES" ] || CAP_SQM_CANDIDATES=""
    CAP_FIREWALL="unknown"
    command -v fw4 >/dev/null 2>&1 && CAP_FIREWALL="firewall4"
    command -v fw3 >/dev/null 2>&1 && CAP_FIREWALL="firewall3"
    CAP_SWITCH="unknown"
    [ -d /sys/class/net ] && { command -v bridge >/dev/null 2>&1 && CAP_SWITCH="dsa"; command -v swconfig >/dev/null 2>&1 && CAP_SWITCH="swconfig"; }
    CAP_PACKAGES=$(opkg list-installed 2>/dev/null | awk '{print $1}' | grep -E '^(wireguard|usteer|sqm|luci-app-sqm|kmod-sched-cake|firewall[34])' | head -n 32 | sed 's/.*/"&"/' | paste -sd, -)
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
    PAYLOAD=$(cat <<EOF
{
    "device_id": "$DEVICE_ID",
    "agent_version": "$AGENT_VERSION",
    "timestamp": $(date +%s),
    "board": $BOARD,
    "system": $SYS_INFO,
    "capabilities": {"openwrt_release":"$CAP_RELEASE","architecture":"$CAP_ARCH","kernel":"$CAP_KERNEL","ram_mb":${CAP_RAM_MB:-0},"flash_mb":${CAP_FLASH_MB:-0},"interfaces":[${CAP_INTERFACES}],"radios":[${CAP_RADIOS}],"wifi_device_sections":[${CAP_WIFI_DEVICES}],"wifi_iface_sections":[${CAP_WIFI_IFACES}],"logical_networks":{${CAP_LOGICAL_NETWORKS}},"sqm_candidates":[${CAP_SQM_CANDIDATES}],"switch_stack":"$CAP_SWITCH","firewall":"$CAP_FIREWALL","packages":[${CAP_PACKAGES}]},
    "wireless_stations": $WIFI_DATA,
    "top_talkers": $TOP_TALKERS,
    "iface_stats": $IFACE_STATS,
    "neighbor_stats": $NEIGHBOR_STATS,
    "dhcp": $DHCP_LEASES,
    "flow_sense": $FLOW_SENSE_DATA,
    "logs": "$SYS_LOGS",
    "transaction": $(transaction_status_json),
    "survey_id": "$SURVEY_ID",
    "neighbor_aps": $NEIGHBOR_APS
}
EOF
)

    # 6. TELEMETRY (using the device token and rollback checks)
    # The device token both routes the request to its tenant and authenticates
    # this enrolled device.
    TELEMETRY_HEADERS="-H X-Device-Token:$DEVICE_TOKEN"
    HTTP_CODE=$(curl -m 5 -s -X POST \
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
    CONFIG_HTTP_CODE=$(curl -m 5 -s -X GET $CONFIG_HEADERS "$CONFIG_URL" -w "%{http_code}" -o "$CONFIG_RESPONSE_FILE")
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
    if [ -z "$PENDING_OPERATION" ] && [ -f "$NERVE_OPERATION_STATUS_FILE" ]; then
        rm -f "$NERVE_OPERATION_STATUS_FILE"
    fi
    if [ -n "$PENDING_OPERATION" ]; then
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
								if [ "$W_AUTH_SERVER" = "AUTO" ]; then W_AUTH_SERVER="$CONTROLLER_IP"; fi

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
            if curl -m 60 -s -H "X-Device-Token: $DEVICE_TOKEN" \
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
