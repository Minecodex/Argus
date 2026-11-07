import { Controller, useFormContext } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Field, Input, Select } from "@argus/ui";
import {
  passwordStrength,
  TIMEZONE_OPTIONS,
  type SetupDraft,
} from "./validation";

export function SetupFields() {
  const { t } = useTranslation();
  const {
    control,
    register,
    getValues,
    setValue,
    watch,
    formState: { errors },
  } = useFormContext<SetupDraft>();

  const password = watch("admin.password");
  const strength = passwordStrength(password);
  const strengthKeys = ["weak", "medium", "strong"] as const;

  return (
    <div className="argus-setup-fields">
      <h3 className="argus-setup-section">
        {t("setup.system.enterpriseSection")}
      </h3>
      <Field
        requirement="required"
        error={errors.enterpriseName?.message}
        label={t("setup.system.enterpriseName.label")}
      >
        <Input
          {...register("enterpriseName")}
          placeholder={t("setup.system.enterpriseName.placeholder")}
        />
      </Field>
      <Field requirement="required" label={t("setup.system.timezone.label")}>
        <Controller
          control={control}
          name="timezone"
          render={({ field }) => (
            <Select
              onValueChange={field.onChange}
              options={TIMEZONE_OPTIONS.map((zone) => ({
                value: zone,
                label: zone,
              }))}
              value={field.value}
            />
          )}
        />
      </Field>
      <h3 className="argus-setup-section">{t("setup.system.adminSection")}</h3>
      <div className="argus-setup-grid-2">
        <Field
          requirement="required"
          error={errors.admin?.username?.message}
          hint={t("setup.system.username.hint")}
          label={t("setup.system.username.label")}
        >
          <Input
            {...register("admin.username", {
              onBlur: () => {
                if (!getValues("admin.displayName").trim()) {
                  setValue(
                    "admin.displayName",
                    getValues("admin.username").trim(),
                    {
                      shouldValidate: true,
                    },
                  );
                }
              },
            })}
            autoComplete="username"
            placeholder={t("setup.system.username.placeholder")}
          />
        </Field>
        <Field
          requirement="required"
          error={errors.admin?.displayName?.message}
          label={t("setup.system.displayName.label")}
        >
          <Input
            {...register("admin.displayName")}
            placeholder={t("setup.system.displayName.placeholder")}
          />
        </Field>
      </div>
      <div className="argus-setup-grid-2">
        <Field
          requirement="required"
          error={errors.admin?.password?.message}
          hint={t("setup.system.password.hint")}
          label={t("setup.system.password.label")}
        >
          <Input
            {...register("admin.password")}
            type="password"
            autoComplete="new-password"
          />
        </Field>
        <Field
          requirement="required"
          error={errors.admin?.confirmPassword?.message}
          label={t("setup.system.confirmPassword.label")}
        >
          <Input
            {...register("admin.confirmPassword")}
            type="password"
            autoComplete="new-password"
          />
        </Field>
      </div>
      {password && (
        <div className={`argus-setup-strength is-${strengthKeys[strength]}`}>
          <span className="argus-setup-strength__label">
            {t("setup.system.strength.label")}：
            {t(`setup.system.strength.${strengthKeys[strength]}`)}
          </span>
          <span className="argus-setup-strength__bars">
            {[0, 1, 2].map((level) => (
              <i
                aria-hidden
                className={level <= strength ? "is-on" : ""}
                key={level}
              />
            ))}
          </span>
        </div>
      )}
    </div>
  );
}
