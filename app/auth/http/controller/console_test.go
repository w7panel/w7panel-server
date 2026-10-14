package controller

// TestConsole_Redirect is intentionally disabled: Console.Redirect currently
// constructs its OAuth client internally and its only observable successful
// path requires a remote Console OAuth service. Keep this test disabled until
// the client is injected, rather than making the suite network-dependent.
