import { useState } from "react";
type Props = {
  loading?: boolean;
  message?: string;
  microsoftEnabled?: boolean;
  onSignIn: (username: string, password: string) => Promise<void>;
  onMicrosoftSignIn?: () => void;
};

export function LoginPage({ loading, message, microsoftEnabled, onSignIn, onMicrosoftSignIn }: Props) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");

  return (
    <div className="login-shell">
      <section className="login-hero">
        <div className="login-copy">
          <p className="eyebrow">Internal access workspace</p>
          <h1>One operational workspace for shared access, controlled use, and audit visibility.</h1>
        </div>

        <div className="login-feature-grid">
          <article className="feature-card">
            <p className="eyebrow">Catalog</p>
            <h2>Discover approved access</h2>
            <p>Find SSH, RDP, portal, and secret entries without relying on chat history or tribal knowledge.</p>
          </article>
          <article className="feature-card">
            <p className="eyebrow">Actions</p>
            <h2>Open, reveal, or launch</h2>
            <p>Use the action that matches the resource while keeping room for stronger policy later.</p>
          </article>
          <article className="feature-card">
            <p className="eyebrow">Audit</p>
            <h2>Track sensitive operations</h2>
            <p>Build toward governed operational access instead of informal sharing and untracked usage.</p>
          </article>
        </div>
      </section>

      <section className="login-panel">
        <div className="login-panel-header">
          <div>
            <p className="eyebrow">Sign in</p>
            <h2>Enter the workspace</h2>
          </div>
          <span className="muted">Local DEV</span>
        </div>

        <p className="section-copy">
          Use a local username and password to enter the workspace. Azure / Entra sign-in can be configured later from Administration.
        </p>

        {message ? <div className="banner compact">{message}</div> : null}

        {microsoftEnabled ? (
          <button className="button microsoft" disabled={loading} onClick={onMicrosoftSignIn}>
            Sign in with Microsoft
          </button>
        ) : null}

        <div className="login-divider">
          <span>Local development login</span>
        </div>

        <div className="form-grid">
          <label className="wide">
            <span>Username</span>
            <input value={username} onChange={(event) => setUsername(event.target.value)} />
          </label>
          <label className="wide">
            <span>Password</span>
            <input
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter" && !loading) {
                  void onSignIn(username, password);
                }
              }}
            />
          </label>
        </div>

        <button className="button primary" disabled={loading || username.trim() === "" || password === ""} onClick={() => void onSignIn(username, password)}>
          {loading ? "Signing in..." : "Sign in"}
        </button>
      </section>
    </div>
  );
}
