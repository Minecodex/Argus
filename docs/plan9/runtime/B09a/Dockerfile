FROM argus-plan9-host:b01-20261026
COPY workload.py /app/workload.py
RUN mkdir -p /data && chmod 1777 /data
ENTRYPOINT ["python3", "/app/workload.py"]
CMD ["serve"]
