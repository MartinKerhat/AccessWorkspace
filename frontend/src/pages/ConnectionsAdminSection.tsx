import type { AdminConfig } from "../types";

type Props = {
  adminConfig: AdminConfig | null;
  rdpSigningEnabled: boolean;
  onRdpSigningEnabledChange: (checked: boolean) => void;
  busy: boolean;
  onSave: () => void;
  onGenerateTestCertificate: () => void;
};

export function ConnectionsAdminSection({
  adminConfig,
  rdpSigningEnabled,
  onRdpSigningEnabledChange,
  busy,
  onSave,
  onGenerateTestCertificate
}: Props) {
  return (
    <div className="admin-grid">
      <section className="panel">
        <div className="panel-header">
          <div>
            <p className="eyebrow">RDP publisher</p>
            <h2>Signed RDP profile trust</h2>
          </div>
          <span className="muted">{adminConfig?.rdpSigning.certificateConfigured ? "Certificate ready" : "No certificate"}</span>
        </div>
        <p className="section-copy">
          Generate a test publisher certificate that the launcher can install into the current Windows user profile and use for signing managed RDP files before MSTSC opens them.
        </p>
        <div className="form-grid">
          <label className="checkbox wide">
            <input
              type="checkbox"
              checked={rdpSigningEnabled}
              onChange={(event) => onRdpSigningEnabledChange(event.target.checked)}
            />
            <span>Enable signed RDP profiles for launcher handoff</span>
          </label>
        </div>
        <dl className="detail-grid compact-detail-grid">
          <div>
            <dt>Status</dt>
            <dd>{adminConfig?.rdpSigning.enabled ? "enabled" : "disabled"}</dd>
          </div>
          <div>
            <dt>Certificate</dt>
            <dd>{adminConfig?.rdpSigning.certificateConfigured ? "configured" : "not generated"}</dd>
          </div>
          <div>
            <dt>Subject</dt>
            <dd>{adminConfig?.rdpSigning.subject || "not set"}</dd>
          </div>
          <div>
            <dt>Thumbprint</dt>
            <dd>{adminConfig?.rdpSigning.thumbprintSha256 || "not set"}</dd>
          </div>
          <div>
            <dt>Generated</dt>
            <dd>{adminConfig?.rdpSigning.generatedAt ? new Date(adminConfig.rdpSigning.generatedAt).toLocaleString() : "never"}</dd>
          </div>
        </dl>
        <div className="action-row">
          <button className="button primary" disabled={busy} onClick={onSave}>
            Save RDP signing settings
          </button>
          <button className="button ghost" disabled={busy} onClick={onGenerateTestCertificate}>
            Generate test certificate
          </button>
        </div>
      </section>
    </div>
  );
}
