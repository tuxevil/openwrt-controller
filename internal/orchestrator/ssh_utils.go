package orchestrator

// This file used to host the TOFU callback and known_hosts mutex. Both
// responsibilities now live in dedicated, single-source-of-truth types:
//
//   - HostKeyManager  (hostkey.go)     — TOFU callback + known_hosts storage
//   - KeyStore        (keystore.go)    — controller private key loader
//
// TofuHostKeyCallback is preserved as a thin wrapper that delegates to the
// process-wide HostKeyManager so the rest of the codebase does not need to
// change. See hostkey.go for the implementation.
