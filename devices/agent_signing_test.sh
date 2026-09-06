#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
AGENT="$SCRIPT_DIR/agent.sh"
BOOTSTRAP="$SCRIPT_DIR/99-nerve-center-bootstrap"

if ! command -v openssl >/dev/null 2>&1; then
    echo "agent signing test skipped: openssl is required"
    exit 0
fi

ROOT=$(mktemp -d)
trap 'rm -rf "$ROOT"' EXIT

printf '%s\n' '#!/bin/sh' 'printf signed' > "$ROOT/artifact"

# Find a valid Ed25519 signature containing a NUL byte. The verifier must keep
# it in a file; command substitution would silently corrupt this input.
attempt=0
while :; do
    openssl genpkey -algorithm ED25519 -out "$ROOT/private.pem" >/dev/null 2>&1
    openssl pkey -in "$ROOT/private.pem" -pubout -outform DER -out "$ROOT/public.der" >/dev/null 2>&1
    openssl pkeyutl -sign -inkey "$ROOT/private.pem" -rawin -in "$ROOT/artifact" -out "$ROOT/signature" >/dev/null 2>&1
    signature_hex=$(od -An -tx1 -v "$ROOT/signature" | tr -d ' \n')
    case "$signature_hex" in
        *00*) break ;;
    esac
    attempt=$((attempt + 1))
    [ "$attempt" -lt 512 ] || {
        echo "could not generate a NUL-containing Ed25519 signature" >&2
        exit 1
    }
done

tail -c 32 "$ROOT/public.der" | base64 | tr -d '\n' > "$ROOT/public.b64"
signature_base64=$(base64 < "$ROOT/signature" | tr -d '\n')
public_key_base64=$(tr -d '\n' < "$ROOT/public.b64")

sh "$AGENT" --self-test-signature "$ROOT/artifact" "$signature_base64" "$public_key_base64"
sh "$BOOTSTRAP" --self-test-signature "$ROOT/artifact" "$signature_base64" "$public_key_base64"

signature_prefix=$(printf '%s' "$signature_base64" | cut -c1-40)
signature_suffix=$(printf '%s' "$signature_base64" | cut -c41-)
signature_with_line_break=$(printf '%s\n%s' "$signature_prefix" "$signature_suffix")
sh "$AGENT" --self-test-signature "$ROOT/artifact" "$signature_with_line_break" "$public_key_base64"
signature_with_trailing_newline=$(printf '%s\nx' "$signature_base64")
signature_with_trailing_newline=${signature_with_trailing_newline%x}
sh "$AGENT" --self-test-signature "$ROOT/artifact" "$signature_with_trailing_newline" "$public_key_base64"

printf '%s\n' '#!/bin/sh' 'printf second-signed' > "$ROOT/artifact-second"
openssl pkeyutl -sign -inkey "$ROOT/private.pem" -rawin -in "$ROOT/artifact-second" -out "$ROOT/signature-second" >/dev/null 2>&1
signature_second=$(base64 < "$ROOT/signature-second" | tr -d '\n')
sh "$AGENT" --self-test-signature "$ROOT/artifact-second" "$signature_second" "$public_key_base64"
sh "$BOOTSTRAP" --self-test-signature "$ROOT/artifact-second" "$signature_second" "$public_key_base64"

if sh "$AGENT" --self-test-signature "$ROOT/artifact" "" "$public_key_base64"; then
    echo "missing signature was accepted" >&2
    exit 1
fi
if sh "$BOOTSTRAP" --self-test-signature "$ROOT/artifact" "" "$public_key_base64"; then
    echo "bootstrap accepted missing signature" >&2
    exit 1
fi

printf '%s\n' modified >> "$ROOT/artifact"
if sh "$AGENT" --self-test-signature "$ROOT/artifact" "$signature_base64" "$public_key_base64"; then
    echo "modified artifact was accepted" >&2
    exit 1
fi
if sh "$BOOTSTRAP" --self-test-signature "$ROOT/artifact" "$signature_base64" "$public_key_base64"; then
    echo "bootstrap accepted modified artifact" >&2
    exit 1
fi

if sh "$AGENT" --self-test-signature "$ROOT/artifact" "not-base64" "$public_key_base64"; then
    echo "malformed signature was accepted" >&2
    exit 1
fi
if sh "$BOOTSTRAP" --self-test-signature "$ROOT/artifact" "not-base64" "$public_key_base64"; then
    echo "bootstrap accepted malformed signature" >&2
    exit 1
fi

echo "agent signing contract passed"
