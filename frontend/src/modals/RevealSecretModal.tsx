type Props = {
  title: string;
  secretValue: string;
  copyMessage?: string;
  onClose: () => void;
  onCopy: () => void;
};

export function RevealSecretModal({ title, secretValue, copyMessage, onClose, onCopy }: Props) {
  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal-card reveal-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <div>
            <p className="eyebrow">Secret reveal</p>
            <h2>{title}</h2>
          </div>
          <button className="button ghost" onClick={onClose}>
            Close
          </button>
        </div>
        {copyMessage ? <div className="banner compact">{copyMessage}</div> : null}
        <button type="button" className="payload-box sensitive clickable" onClick={onCopy}>
          <p className="eyebrow">Secret value</p>
          <pre>{secretValue}</pre>
        </button>
      </div>
    </div>
  );
}
