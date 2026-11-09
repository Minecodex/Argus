import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";

export type CollectorImageValues = { image: string; pullSecrets: string };
const names = (value: string) =>
  value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);

export function useCollectorImageForm(messages: {
  image: string;
  pullSecrets: string;
}) {
  const schema = z.object({
    image: z
      .string()
      .trim()
      .max(512, messages.image)
      .refine((value) => !value || /^\S+:\S+$/.test(value), messages.image),
    pullSecrets: z.string().refine((value) => {
      const items = names(value);
      return (
        items.length <= 16 &&
        new Set(items).size === items.length &&
        items.every(
          (item) =>
            item.length <= 253 && /^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$/.test(item),
        )
      );
    }, messages.pullSecrets),
  });
  return useForm<CollectorImageValues>({
    resolver: zodResolver(schema),
    mode: "onChange",
    defaultValues: { image: "", pullSecrets: "" },
  });
}

export function collectorImageOptions(value: CollectorImageValues) {
  const image = value.image.trim();
  const imagePullSecrets = names(value.pullSecrets);
  return {
    ...(image ? { kubernetes_image: image } : {}),
    ...(imagePullSecrets.length
      ? { image_pull_secrets: imagePullSecrets }
      : {}),
  };
}
