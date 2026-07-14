import { ResourceFormCard } from "../components/ResourceForm";
import type { Resource, ResourceForm, UserSummary } from "../types";

type Props = {
  mode: "create" | "edit";
  headingName: string;
  resource?: Resource;
  initialType?: ResourceForm["type"];
  availableGroups: string[];
  availableOwners: UserSummary[];
  defaultPersonalPassword: boolean;
  canAssignOwner: boolean;
  sharedMetadataOnly?: boolean;
  loading: boolean;
  onSubmit: (input: ResourceForm) => Promise<void>;
  onRevealStoredPassword?: () => Promise<string | undefined>;
  onArchive?: () => Promise<void>;
  onClose: () => void;
};

export function ResourceFormModal({
  mode,
  headingName,
  resource,
  initialType,
  availableGroups,
  availableOwners,
  defaultPersonalPassword,
  canAssignOwner,
  sharedMetadataOnly = false,
  loading,
  onSubmit,
  onRevealStoredPassword,
  onArchive,
  onClose
}: Props) {
  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal-card" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">{mode === "create" ? "Create object" : "Edit object"}</p>
            <h2>{headingName}</h2>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>
        <ResourceFormCard
          resource={resource}
          initialType={initialType}
          availableGroups={availableGroups}
          availableOwners={availableOwners}
          defaultPersonalPassword={defaultPersonalPassword}
          canAssignOwner={canAssignOwner}
          sharedMetadataOnly={sharedMetadataOnly}
          loading={loading}
          onSubmit={onSubmit}
          onRevealStoredPassword={onRevealStoredPassword}
          onArchive={onArchive}
        />
      </div>
    </div>
  );
}
