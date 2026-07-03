import type { User } from "../types";

type Props = {
  user: User;
};

export function DiagnosticsAdminSection({ user }: Props) {
  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <p className="eyebrow">Diagnostics</p>
          <h2>Current session authorization</h2>
        </div>
      </div>
      <p className="section-copy">
        This remains available for QA and troubleshooting, but it has been moved out of the normal account bubble.
      </p>
      <div className="detail-grid">
        <div>
          <dt>Azure groups</dt>
          <dd>{user.groups.length}</dd>
        </div>
        <div>
          <dt>Local groups</dt>
          <dd>{user.localGroups.length}</dd>
        </div>
        <div>
          <dt>Rights</dt>
          <dd>{user.rights.length}</dd>
        </div>
      </div>
      {user.groups.length > 0 ? (
        <div className="group-card-section">
          <p className="eyebrow">Azure groups</p>
          <div className="tag-row">
            {user.groups.map((group) => (
              <span key={group} className="tag">
                {group}
              </span>
            ))}
          </div>
        </div>
      ) : null}
      {user.localGroups.length > 0 ? (
        <div className="group-card-section">
          <p className="eyebrow">Local groups</p>
          <div className="tag-row">
            {user.localGroups.map((group) => (
              <span key={group} className="tag">
                {group}
              </span>
            ))}
          </div>
        </div>
      ) : null}
      {user.rights.length > 0 ? (
        <div className="group-card-section">
          <p className="eyebrow">Rights</p>
          <div className="tag-row">
            {user.rights.map((right) => (
              <span key={right} className="tag">
                {right}
              </span>
            ))}
          </div>
        </div>
      ) : null}
    </section>
  );
}
