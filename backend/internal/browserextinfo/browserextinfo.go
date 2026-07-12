package browserextinfo

// RequiredVersion is the browser-extension protocol version the backend expects.
// Downloadable packages themselves are enumerated dynamically by the artifacts
// package; this constant only gates the extension connect/handshake flow.
const RequiredVersion = "0.2.8"
