import { expect, it } from "vitest";
import { dashboardKubernetesScope, dashboardKubernetesSearch, parseKubernetesDashboardScope } from "./url-kubernetes-scope";
it("round trips multiple authorized namespace ids without widening to the cluster",()=>{
 const selected={cluster_id:"cluster",namespace_ids:["one","two"]};
 const url=new URLSearchParams(dashboardKubernetesSearch(selected));
 const fixed=dashboardKubernetesScope(parseKubernetesDashboardScope(Object.fromEntries(url)));
 expect(fixed?.kubernetes_scope).toEqual(selected);
 expect(fixed?.resource_ids).toEqual(["one","two"]);
 expect(fixed?.resource_types).toEqual(["kubernetes_namespace"]);
});
it("retains invalid references for server rejection instead of falling back to a broader scope",()=>{
 expect(dashboardKubernetesScope({cluster_scope:"cluster",namespace_scope:"invalid,,two"})?.resource_ids).toEqual(["invalid","","two"]);
 expect(dashboardKubernetesScope({cluster_scope:"cluster",namespace_scope:"one"})?.resource_ids).toEqual(["one"]);
});
