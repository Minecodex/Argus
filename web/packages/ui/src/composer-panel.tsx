import type { ReactNode } from "react";
import { ChevronLeft, ChevronRight, X } from "lucide-react";
import { Button } from "./button";
export function ComposerPanel({
  title,
  progress,
  onPrevious,
  onNext,
  onClose,
  children,
  footer,
  labels,
}: {
  title: string;
  progress?: string;
  onPrevious?: () => void;
  onNext?: () => void;
  onClose(): void;
  children: ReactNode;
  footer: ReactNode;
  labels: { close: string; previous: string; next: string };
}) {
  return (
    <section className="argus-composer-panel" aria-label={title}>
      <header className="argus-composer-panel__header">
        <strong>{title}</strong>
        <div className="argus-composer-panel__navigation">
          {progress && (
            <>
              <Button
                variant="ghost"
                size="sm"
                isIconOnly
                aria-label={labels.previous}
                isDisabled={!onPrevious}
                onPress={onPrevious}
              >
                <ChevronLeft />
              </Button>
              <span aria-live="polite">{progress}</span>
              <Button
                variant="ghost"
                size="sm"
                isIconOnly
                aria-label={labels.next}
                isDisabled={!onNext}
                onPress={onNext}
              >
                <ChevronRight />
              </Button>
            </>
          )}
          <Button
            variant="ghost"
            size="sm"
            isIconOnly
            aria-label={labels.close}
            onPress={onClose}
          >
            <X />
          </Button>
        </div>
      </header>
      <div className="argus-composer-panel__body">{children}</div>
      <footer className="argus-composer-panel__footer">{footer}</footer>
    </section>
  );
}
export function QuestionChoices({
  name,
  label,
  options,
  selected,
  onSelect,
  disabled,
}: {
  name: string;
  label: string;
  options: { label: string; description: string }[];
  selected?: string;
  onSelect(value: string): void;
  disabled?: boolean;
}) {
  return (
    <div
      role="radiogroup"
      aria-label={label}
      className="argus-question-choices"
    >
      {options.map((option) => (
        <label
          key={option.label}
          className={`argus-question-choices__option${selected === option.label ? " is-selected" : ""}`}
        >
          <input
            type="radio"
            name={name}
            value={option.label}
            checked={selected === option.label}
            disabled={disabled}
            onChange={() => onSelect(option.label)}
          />
          <span>
            <strong>{option.label}</strong>
            <span className="argus-question-choices__description">
              {option.description}
            </span>
          </span>
        </label>
      ))}
    </div>
  );
}
export function QuestionAnswerSummary({
  label,
  items,
}: {
  label: string;
  items: { question: string; answer: string }[];
}) {
  return (
    <details className="argus-question-summary">
      <summary>{label}</summary>
      <dl>
        {items.map((item, index) => (
          <div key={index}>
            <dt>{item.question}</dt>
            <dd>{item.answer}</dd>
          </div>
        ))}
      </dl>
    </details>
  );
}
