"""Small real application Chart with a bounded post-install/upgrade Hook."""
from pathlib import Path
import shutil
import yaml
from infrastructure import HERE, execute

TEMPLATE = '''apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}-code
  labels: {argus.plan9/run: {{ .Values.run | quote }}}
data:
  service.py: |
{{ .Files.Get "files/service.py" | indent 4 }}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}
  labels: {argus.plan9/run: {{ .Values.run | quote }}}
spec:
  strategy: {type: Recreate}
  replicas: {{ .Values.replicas }}
  selector:
    matchLabels: {app: {{ .Release.Name }}}
  template:
    metadata:
      labels: {app: {{ .Release.Name }}, argus.plan9/run: {{ .Values.run | quote }}}
    spec:
      automountServiceAccountToken: false
      nodeSelector: {argus.plan9/role: worker}
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels: {app: {{ .Release.Name }}}
              topologyKey: kubernetes.io/hostname
      volumes:
      - name: code
        configMap: {name: {{ .Release.Name }}-code}
      containers:
      - name: web
        image: {{ .Values.image | quote }}
        imagePullPolicy: Never
        command: [python3, -u, /batch/service.py]
        ports: [{containerPort: 8080}]
        volumeMounts: [{name: code, mountPath: /batch, readOnly: true}]
        env:
        - {name: PLAN9_RUN, value: {{ .Values.run | quote }}}
        - {name: CHART_VERSION, value: {{ .Chart.Version | quote }}}
        - {name: PLAN9_FAULT, value: {{ .Values.fault | quote }}}
        - {name: FAULT_UNTIL, value: {{ .Values.faultUntil | quote }}}
        - name: POD_NAME
          valueFrom: {fieldRef: {fieldPath: metadata.name}}
        readinessProbe:
          httpGet: {path: /ready, port: 8080}
          periodSeconds: 3
          failureThreshold: 1
        resources:
          requests: {cpu: 10m, memory: 24Mi}
          limits: {cpu: 150m, memory: 96Mi}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}
  labels: {argus.plan9/run: {{ .Values.run | quote }}}
spec:
  selector: {app: {{ .Release.Name }}}
  ports: [{port: 8080, targetPort: 8080}]
---
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ .Release.Name }}-hook
  labels: {argus.plan9/run: {{ .Values.run | quote }}}
  annotations:
    helm.sh/hook: post-install,post-upgrade,post-rollback
    helm.sh/hook-delete-policy: before-hook-creation
spec:
  backoffLimit: 0
  activeDeadlineSeconds: 30
  template:
    metadata:
      labels: {argus.plan9/run: {{ .Values.run | quote }}}
    spec:
      automountServiceAccountToken: false
      restartPolicy: Never
      nodeSelector: {argus.plan9/role: worker}
      containers:
      - name: hook
        image: {{ .Values.image | quote }}
        imagePullPolicy: Never
        command: [python3, -c, "print('B10C_HOOK_OK')"]
        resources:
          requests: {cpu: 5m, memory: 16Mi}
          limits: {cpu: 100m, memory: 64Mi}
'''


def package(run):
    output = run.out/'charts'
    output.mkdir(exist_ok=True)
    for version in ('0.1.0','0.2.0'):
        chart = run.out/('chart-'+version)/'b10c-web'
        (chart/'templates').mkdir(parents=True, exist_ok=True)
        (chart/'files').mkdir(exist_ok=True)
        (chart/'Chart.yaml').write_text(yaml.safe_dump({'apiVersion':'v2','name':'b10c-web','version':version,'type':'application'}), encoding='utf-8')
        (chart/'values.yaml').write_text(yaml.safe_dump({'image':run.image,'replicas':2,'run':run.id,'fault':'0','faultUntil':'0'}), encoding='utf-8')
        (chart/'templates/workload.yaml').write_text(TEMPLATE, encoding='utf-8')
        shutil.copy2(HERE/'service.py',chart/'files/service.py')
        execute(['helm','lint',str(chart)])
        execute(['helm','package',str(chart),'--destination',str(output)])
        assert (output/('b10c-web-'+version+'.tgz')).stat().st_size < 393216
    execute(['helm','repo','index',str(output),'--url','https://'+run.source_addresses()['repository']+':8443'])
