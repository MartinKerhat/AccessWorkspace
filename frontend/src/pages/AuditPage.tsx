import { useEffect, useRef, useState } from "react";
import type { AuditEvent } from "../types";

type Props = {
  items: AuditEvent[];
  total: number;
  eventTypes: string[];
  hasMore?: boolean;
  onLoadOlder?: () => void;
  onFiltersChange?: (filters: { query: string; eventType: string }) => void;
};

// Filtering happens server-side over the FULL audit history; this page only
// debounces the inputs and renders whatever pages the app has loaded so far.
export function AuditPage({ items, total, eventTypes, hasMore = false, onLoadOlder, onFiltersChange }: Props) {
  const [query, setQuery] = useState("");
  const [eventType, setEventType] = useState("");
  const skipInitialFilterEffect = useRef(true);

  useEffect(() => {
    if (skipInitialFilterEffect.current) {
      skipInitialFilterEffect.current = false;
      return;
    }
    const handle = window.setTimeout(() => {
      onFiltersChange?.({ query, eventType });
    }, 300);
    return () => window.clearTimeout(handle);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, eventType]);

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <p className="eyebrow">Audit</p>
          <h2>Audit trail</h2>
        </div>
        <span className="muted">
          {items.length < total ? `Showing ${items.length} of ${total} events` : `${total} events`}
        </span>
      </div>

      <p className="section-copy">
        Review who changed, revealed, launched, or viewed objects across the workspace. Search covers the entire history.
      </p>

      <div className="filter-grid audit-filter-grid">
        <input
          placeholder="Search by user, object, id, event"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
        <select value={eventType} onChange={(event) => setEventType(event.target.value)}>
          <option value="">All event types</option>
          {eventTypes.map((value) => (
            <option key={value} value={value}>
              {value}
            </option>
          ))}
        </select>
      </div>

      <div className="activity-list">
        {items.map((item) => (
          <article key={item.id} className="activity-item">
            <div>
              <strong>{item.eventType}</strong>
              <p>
                {item.resourceName ?? "No object"} by {item.userName}
              </p>
            </div>
            <time>{new Date(item.createdAt).toLocaleString()}</time>
          </article>
        ))}
        {items.length === 0 ? <p className="detail-description">No events match the current filter.</p> : null}
      </div>

      {hasMore && onLoadOlder ? (
        <div className="action-row compact-actions">
          <button className="button ghost" onClick={onLoadOlder}>
            Load older events
          </button>
        </div>
      ) : null}
    </section>
  );
}
