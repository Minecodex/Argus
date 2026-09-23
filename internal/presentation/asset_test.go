package presentation

import "testing"

func TestTemplateAssetRejectsCapabilityAndHashChanges(t *testing.T) {
 for _,source:=range []string{`<script src="remote.js"></script>`,`<form></form>`,`<iframe></iframe>`,`<script>eval("x")</script>`} {if Validate(Asset(source,"v1"))==nil {t.Fatalf("accepted %q",source)}}
 asset:=Asset(`<p>Details</p><script>ArgusTemplate.onData(()=>{});</script>`,"v1")
 if err:=Validate(asset);err!=nil{t.Fatal(err)}
 asset.Source+="changed";if Validate(asset)==nil{t.Fatal("accepted hash mismatch")}
}
func TestDetailsNeverCarriesActionAuthority(t *testing.T) {
 original:=map[string]any{"action_ref":"private-reference","diff":map[string]any{"name":"new","argus__token":"secret"}}
 detail:=Details(original)
 if _,ok:=detail["action_ref"];ok{t.Fatal("control reference leaked")}
 if _,ok:=detail["diff"].(map[string]any)["argus__token"];ok{t.Fatal("nested authority leaked")}
 if original["action_ref"]==nil{t.Fatal("projection mutated original result")}
}
