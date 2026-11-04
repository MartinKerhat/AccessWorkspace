import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { KeyVaultSecretVersion, Resource } from "../types";

// Long-rotated secrets can have hundreds of versions; the newest few are what
// people look at.
const VISIBLE_VERSIONS = 10;

function formatDate(value?: string) {
  if (!value) {
    return "—";
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

type Props = {
  resource: Resource;
  loading?: boolean;
  canReveal: boolean;
  onSetValue?: () => void;
  onEditVersion?: (version: KeyVaultSecretVersion) => void;
  onRevealVersion?: (version: string) => void;
};

// The versions of an Azure Key Vault secret, newest first, read live from
// Azure on every selection or refetch of the object. Key Vault cannot delete a
// single version, so a version can only be revealed, enabled/disabled, or have
// its active window changed.
export function KeyVaultVersions({ resource, loading, canReveal, onSetValue, onEditVersion, onRevealVersion }: Props) {
  const [versions, setVersions] = useState<KeyVaultSecretVersion[]>([]);
  const [error, setError] = useState("");
  const [fetching, setFetching] = useState(false);
  const [showAll, setShowAll] = useState(false);
  const [copied, setCopied] = useState("");

  useEffect(() => {
    let cancelled = false;
    setFetching(true);
    setError("");
    api
      .listKeyVaultSecretVersions(resource.id)
      .then((response) => {
        if (!cancelled) {
          setVersions(response.items);
        }
      })
      .catch((reason) => {
        if (!cancelled) {
          setVersions([]);
          setError(reason instanceof Error ? reason.message : "Loading the versions failed");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setFetching(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [resource]);

  useEffect(() => {
    setShowAll(false);
  }, [resource.id]);

  async function copyId(version: string) {
    try {
      await navigator.clipboard.writeText(version);
      setCopied(version);
    } catch {
      setCopied("");
    }
  }

  const visible = showAll ? versions : versions.slice(0, VISIBLE_VERSIONS);

  return (
    <div className="detail-section">
      <div className="keyvault-versions-header">
        <p className="eyebrow">Versions{versions.length > 0 ? ` (${versions.length})` : ""}</p>
        {onSetValue ? (
          <button className="button ghost" disabled={loading} onClick={onSetValue}>
            Set new value
          </button>
        ) : null}
      </div>
      {error ? <p className="detail-description error-copy">{error}</p> : null}
      {fetching && versions.length === 0 && !error ? <p className="detail-description">Loading versions from Azure…</p> : null}
      <div className="credential-list">
        {visible.map((item) => (
          <div key={item.version} className="credential-row keyvault-version-row">
            <div>
              <strong>
                <button
                  type="button"
                  className="keyvault-version-id"
                  title={`${item.version} — click to copy`}
                  onClick={() => void copyId(item.version)}
                >
                  {item.version.slice(0, 8)}
                </button>
                {item.current ? <span className="tag">current</span> : null}
                {item.pinned ? <span className="tag">pinned</span> : null}
                {copied === item.version ? <span className="selection-hint">copied</span> : null}
              </strong>
              <p>
                Created {formatDate(item.createdAt)}
                {item.createdBy ? ` by ${item.createdBy}` : ""} · from {formatDate(item.notBefore)} · until {formatDate(item.expiresAt)}
              </p>
            </div>
            <div className="keyvault-version-actions">
              <span className={`tag ${item.enabled ? "credential-state-active" : "credential-state-expired"}`}>
                {item.enabled ? "Enabled" : "Disabled"}
              </span>
              {canReveal && onRevealVersion ? (
                <button
                  className="button ghost"
                  disabled={loading || !item.enabled}
                  title={item.enabled ? undefined : "Azure refuses to read a disabled version"}
                  onClick={() => onRevealVersion(item.version)}
                >
                  Reveal
                </button>
              ) : null}
              {onEditVersion ? (
                <button className="button ghost" disabled={loading} onClick={() => onEditVersion(item)}>
                  Edit
                </button>
              ) : null}
            </div>
          </div>
        ))}
      </div>
      {versions.length > VISIBLE_VERSIONS ? (
        <div className="action-row compact-actions">
          <button className="button ghost" onClick={() => setShowAll((current) => !current)}>
            {showAll ? "Show newest only" : `Show all (${versions.length})`}
          </button>
        </div>
      ) : null}
    </div>
  );
}
