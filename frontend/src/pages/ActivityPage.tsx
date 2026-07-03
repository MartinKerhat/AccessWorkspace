import type { AuditEvent } from "../types";

type Props = {
  title: string;
  description: string;
  items: AuditEvent[];
};

export function ActivityPage({ title, description, items }: Props) {
  return (
    <section className="panel">
      <div className="panel-header">
        <div>
          <p className="eyebrow">Activity</p>
          <h2>{title}</h2>
        </div>
        <span className="muted">{items.length} events</span>
      </div>

      <p className="section-copy">{description}</p>

      <div className="activity-list">
        {items.map((item) => (
          <article key={item.id} className="activity-item">
            <div>
              <strong>{item.eventType}</strong>
              <p>{item.resourceName ?? "No resource"} by {item.userName}</p>
            </div>
            <time>{new Date(item.createdAt).toLocaleString()}</time>
          </article>
        ))}
      </div>
    </section>
  );
}
