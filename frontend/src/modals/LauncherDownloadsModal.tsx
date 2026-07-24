import type { LauncherRuntime } from "../types";
import { formatArtifactMeta } from "../format";
import { detectClientLauncherPlatform, matchesLauncherPlatform } from "../platform";

type Props = {
  runtime: LauncherRuntime;
  onClose: () => void;
};

export function LauncherDownloadsModal({ runtime, onClose }: Props) {
  // Builds for the OS this browser runs on come first, tagged; the rest stay
  // available for downloading onto another machine.
  const clientPlatform = detectClientLauncherPlatform();
  const downloads = [...runtime.downloads].sort((a, b) => {
    const aMatch = matchesLauncherPlatform(a, clientPlatform) ? 0 : 1;
    const bMatch = matchesLauncherPlatform(b, clientPlatform) ? 0 : 1;
    return aMatch - bMatch;
  });
  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal-card browser-extension-manager-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">Connection launcher</p>
            <h2>Download launcher</h2>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>
        <div className="detail-section">
          <p className="detail-description">
            Download and install the launcher for your platform, then start connections from the web app. The
            launcher registers the <code>access-workspace://</code> handler on your machine.
          </p>
          <p className="detail-description">Required version: {runtime.requiredVersion}</p>
        </div>
        {downloads.length > 0 ? (
          <ul className="artifact-file-list">
            {downloads.map((file) => (
              <li key={file.name} className="artifact-file">
                <span className="artifact-file-info">
                  <span className="artifact-file-name">
                    {file.name}
                    {matchesLauncherPlatform(file, clientPlatform) ? <span className="tag">for this device</span> : null}
                  </span>
                  {formatArtifactMeta(file) ? (
                    <span className="artifact-file-meta muted">{formatArtifactMeta(file)}</span>
                  ) : null}
                </span>
                <a className="button secondary launch-link-button artifact-download-button" href={file.downloadUrl}>
                  Download
                </a>
              </li>
            ))}
          </ul>
        ) : (
          <p className="detail-description muted">No launcher builds are published yet.</p>
        )}
      </div>
    </div>
  );
}
