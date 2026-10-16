import fs from "node:fs";
const source=JSON.parse(fs.readFileSync("api/openapi/components/alerts.yaml","utf8"));
const definitions=Object.fromEntries(Object.entries(source.components.schemas).filter(([key])=>!['ApiError','PendingActionPublic'].includes(key)));
const contract={$schema:'https://json-schema.org/draft/2020-12/schema',$id:'https://argus.io/schemas/v1/alerts/alerts.schema.json',$ref:'#/$defs/AlertPolicySpec',$defs:JSON.parse(JSON.stringify(definitions).replaceAll('#/components/schemas/','#/$defs/'))};
fs.writeFileSync('api/schemas/alerts/alerts.schema.json',JSON.stringify(contract,null,2)+'\n');
const template=fs.readFileSync('api/schemas/alerts/result.html','utf8');
fs.writeFileSync('web/packages/api-client/src/generated/alert-result-template.ts','// Generated from api/schemas/alerts/result.html; do not edit.\nexport const alertResultTemplate = '+JSON.stringify(template)+';\n');
