import { Button } from "@argus/ui";

export function HostRemovalActions({
  busy,
  manual,
  onRetry,
  onRegenerate,
  retryLabel,
  regenerateLabel,
}: {
  busy: boolean;
  manual: boolean;
  onRetry: () => void;
  onRegenerate: () => void;
  retryLabel: string;
  regenerateLabel: string;
}) {
  return (
    <div className="argus-form-actions">
      <Button
        isDisabled={busy}
        onPress={onRetry}
        type="button"
        variant="primary"
      >
        {retryLabel}
      </Button>
      {manual && (
        <Button
          isDisabled={busy}
          onPress={onRegenerate}
          type="button"
          variant="secondary"
        >
          {regenerateLabel}
        </Button>
      )}
    </div>
  );
}
