import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
/** Draft forms validate author-entered structure; runtime/query validation remains server-side. */
export function useDefinitionForm<T>(
  value: T,
  validate: (value: T) => boolean,
  message: string,
) {
  const form = useForm({
    values: { definition: value },
    resolver: zodResolver(
      z.object({
        definition: z.custom<T>(
          (data) =>
            data !== null && typeof data === "object" && validate(data as T),
          { message },
        ),
      }),
    ),
  });
  return {
    handleSubmit: (save: (value: T) => void | Promise<void>) =>
      form.handleSubmit(({ definition }) => save(definition)),
    error: form.formState.errors.definition?.message as string | undefined,
  };
}
