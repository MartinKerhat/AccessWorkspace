import { useMemo, useState } from "react";
import type { AuditEvent } from "../types";

type Props = {
  items: AuditEvent[];
};

export function AuditPage({ items }: Props) {
  const [query, setQuery] = useState("");
  const [eventType, setEventType] = useState("");

  const eventTypes = useMemo(
    () => Array.from(new Set(items.map((item) => item.eventType))).sort(),
    [items]
  );

  const filteredItems = items.filter((item) => {
    const normalizedQuery = query.trim().toLowerCase();
    const matchesQuery =
      normalizedQuery === "" ||
      item.userName.toLowerCase().includes(normalizedQuery) ||
      (item.resourceName ?? "").toLowerCase().includes(normalizedQuery) ||
      (item.resourceId ?? "").toLowerCase().includes(normalizedQuery) ||
      item.eventType.toLowerCase().includes(normalizedQuery);

    const matchesEventType = eventType === "" || item.eventType === eventType;
    return matchesQuery && matchesEventType;
  });

  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <p className="eyebrow">Audit</p>
          <h2>Audit trail</h2>
        </div>
        <span className="muted">{filteredItems.length} events</span>
      </div>

      <p className="section-copy">
        Review who changed, revealed, launched, or viewed objects across the workspace.
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
        {filteredItems.map((item) => (
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
      </div>
    </section>
  );
}
