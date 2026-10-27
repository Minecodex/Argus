package collectormanager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// All bootstrap accounts below exist only in owned disposable containers.
// The Collector receives a distinct read-only monitoring identity.
const databaseTestPassword = "Plan8-ReadOnly-72!"

func TestNativeMongoDBTarget(t *testing.T) {
	f := newNativeResourceFixture(t)
	script := `db = db.getSiblingDB("admin");
db.createUser({user:"monitor",pwd:"` + databaseTestPassword + `",roles:[{role:"clusterMonitor",db:"admin"}]});
db = db.getSiblingDB("plan8");
db.samples.insertMany([{value:1},{value:2}]);`
	path := f.write("init.js", []byte(script))
	id := f.start("mongodb", "--memory=768m", "--cpus=1", "-e", "MONGO_INITDB_ROOT_USERNAME=bootstrap", "-e", "MONGO_INITDB_ROOT_PASSWORD="+databaseTestPassword,
		"--mount", "type=bind,source="+path+",target=/docker-entrypoint-initdb.d/init.js,readonly", "mongo:7.0.16", "--wiredTigerCacheSizeGB", "0.25")
	args := []string{"exec", id, "mongosh", "--quiet", "--username", "monitor", "--password", databaseTestPassword, "--authenticationDatabase", "admin", "--eval"}
	waitDatabaseReady(t, f, 2*time.Minute, append(slices.Clone(args), `db.adminCommand({serverStatus:1}).ok`)...)
	if _, err := f.command("", append(slices.Clone(args), `db.getSiblingDB("plan8").forbidden.insertOne({value:1})`)...); err == nil {
		t.Fatal("MongoDB monitor could write application data")
	}
	f.add("mongodb", f.owner+"-mongodb:27017", "", map[string]string{"username": "monitor", "password": databaseTestPassword})
	f.collections[0].Config = json.RawMessage(`{"interval":"2s","timeout":"2s","auth_source":"admin","direct_connection":true}`)
	verifyDatabaseTarget(t, f, "mongodb", []string{"mongodb.connection.count", "mongodb.memory.usage", "mongodb.data.size", "mongodb.storage.size", "mongodb.operation.count", "mongodb.network.io.receive"})
}

func TestNativeElasticsearchTarget(t *testing.T) {
	f := newNativeResourceFixture(t)
	id := f.start("elasticsearch", "--memory=1536m", "--cpus=2", "-e", "discovery.type=single-node", "-e", "xpack.security.enabled=true",
		"-e", "xpack.security.http.ssl.enabled=false", "-e", "xpack.security.transport.ssl.enabled=false", "-e", "ES_JAVA_OPTS=-Xms512m -Xmx512m",
		"-e", "ELASTIC_PASSWORD="+databaseTestPassword, "docker.elastic.co/elasticsearch/elasticsearch:8.17.0")
	args := []string{"exec", id, "curl", "-fsS", "-u", "elastic:" + databaseTestPassword, "-H", "Content-Type: application/json"}
	waitDatabaseReady(t, f, 2*time.Minute, append(slices.Clone(args), "http://localhost:9200/_cluster/health")...)
	f.must("", append(slices.Clone(args), "-X", "PUT", "http://localhost:9200/_security/role/plan8_monitor", "-d", `{"cluster":["monitor"],"indices":[{"names":["*"],"privileges":["monitor"]}]}`)...)
	f.must("", append(slices.Clone(args), "-X", "PUT", "http://localhost:9200/_security/user/monitor", "-d", `{"password":"`+databaseTestPassword+`","roles":["plan8_monitor"]}`)...)
	f.must("", append(slices.Clone(args), "-X", "PUT", "http://localhost:9200/plan8", "-d", `{"settings":{"number_of_shards":1,"number_of_replicas":0}}`)...)
	f.must("", append(slices.Clone(args), "-X", "POST", "http://localhost:9200/plan8/_doc?refresh=true", "-d", `{"value":1}`)...)
	if _, err := f.command("", "exec", id, "curl", "-fsS", "-u", "monitor:"+databaseTestPassword, "-X", "PUT", "http://localhost:9200/forbidden"); err == nil {
		t.Fatal("Elasticsearch monitor could create an index")
	}
	f.add("elasticsearch", "http://"+f.owner+"-elasticsearch:9200", "", map[string]string{"username": "monitor", "password": databaseTestPassword})
	verifyDatabaseTarget(t, f, "elasticsearch", []string{"elasticsearch.cluster.health", "elasticsearch.cluster.nodes", "elasticsearch.index.documents", "jvm.memory.heap.used", "elasticsearch.node.fs.disk.available", "elasticsearch.index.operations.completed"})
}

func TestNativeSQLServerTarget(t *testing.T) {
	f := newNativeResourceFixture(t)
	id := f.start("sqlserver", "--memory=2560m", "--cpus=2", "-e", "ACCEPT_EULA=Y", "-e", "MSSQL_PID=Developer", "-e", "MSSQL_MEMORY_LIMIT_MB=1536",
		"-e", "MSSQL_SA_PASSWORD="+databaseTestPassword, "mcr.microsoft.com/mssql/server:2022-CU18-ubuntu-22.04")
	args := []string{"exec", id, "/opt/mssql-tools18/bin/sqlcmd", "-C", "-b", "-S", "localhost", "-U", "sa", "-P", databaseTestPassword, "-Q"}
	waitDatabaseReady(t, f, 2*time.Minute, append(slices.Clone(args), "SELECT 1")...)
	f.must("", append(slices.Clone(args), "CREATE DATABASE plan8; CREATE LOGIN monitor WITH PASSWORD = '"+databaseTestPassword+"', CHECK_POLICY = OFF; GRANT VIEW ANY DATABASE TO monitor; GRANT VIEW SERVER PERFORMANCE STATE TO monitor;")...)
	if _, err := f.command("", "exec", id, "/opt/mssql-tools18/bin/sqlcmd", "-C", "-b", "-S", "localhost", "-U", "monitor", "-P", databaseTestPassword, "-Q", "CREATE DATABASE forbidden"); err == nil {
		t.Fatal("SQL Server monitor could create a database")
	}
	f.add("sqlserver", f.owner+"-sqlserver:1433", "", map[string]string{"username": "monitor", "password": databaseTestPassword})
	// Native SQL rate scrapers have internal sample windows; allow them to
	// finish without changing any receiver implementation.
	f.collections[0].Config = json.RawMessage(`{"interval":"5s","timeout":"5s"}`)
	verifyDatabaseTarget(t, f, "sqlserver", []string{"sqlserver.user.connection.count", "sqlserver.batch.request.rate", "sqlserver.page.buffer_cache.hit_ratio", "sqlserver.page.life_expectancy", "sqlserver.lock.wait.rate", "sqlserver.batch.sql_compilation.rate"})
}

func TestNativeOracleTarget(t *testing.T) {
	f := newNativeResourceFixture(t)
	id := f.start("oracle", "--memory=2560m", "--cpus=2", "-e", "ORACLE_PASSWORD="+databaseTestPassword, "gvenzl/oracle-free:23.5-slim-faststart")
	waitDatabaseReady(t, f, 4*time.Minute, "exec", id, "healthcheck.sh")
	sql := "WHENEVER SQLERROR EXIT FAILURE;\nALTER SESSION SET CONTAINER=FREEPDB1;\nCREATE USER monitor IDENTIFIED BY \"" + databaseTestPassword + "\";\nGRANT CREATE SESSION TO monitor;\n"
	for _, view := range []string{"V_$INSTANCE", "V_$DATABASE", "V_$DATAFILE", "V_$PDBS", "CDB_SERVICES", "V_$SESSION", "V_$SYSSTAT", "V_$RESOURCE_LIMIT", "V_$OSSTAT", "DBA_TABLESPACES", "DBA_DATA_FILES", "DBA_TABLESPACE_USAGE_METRICS", "V_$SGAINFO"} {
		sql += "GRANT SELECT ON SYS." + view + " TO monitor;\n"
	}
	sql += "EXIT;\n"
	f.must(sql, "exec", "-i", id, "sqlplus", "-s", "/", "as", "sysdba")
	if _, err := f.command("WHENEVER SQLERROR EXIT FAILURE;\nCREATE TABLE forbidden (id NUMBER);\nEXIT;\n", "exec", "-i", id, "sqlplus", "-s", "monitor/"+databaseTestPassword+"@//localhost:1521/FREEPDB1"); err == nil {
		t.Fatal("Oracle monitor could create application tables")
	}
	f.add("oracle", f.owner+"-oracle:1521", "FREEPDB1", map[string]string{"username": "monitor", "password": databaseTestPassword})
	f.collections[0].Config = json.RawMessage(`{"interval":"5s","timeout":"5s"}`)
	verifyDatabaseTarget(t, f, "oracle", []string{"oracledb.sessions.usage", "oracledb.cpu_time", "oracledb.pga_memory", "oracledb.tablespace_size.usage", "oracledb.tablespace_size.limit", "oracledb.user_commits"})
}

func waitDatabaseReady(t *testing.T, f *nativeResourceFixture, timeout time.Duration, args ...string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := f.command("", args...); err == nil {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("owned database did not become ready within its startup budget")
}

func verifyDatabaseTarget(t *testing.T, f *nativeResourceFixture, key string, required []string) {
	t.Helper()
	first := f.run(1)
	f.wait(first, 1)
	time.Sleep(12 * time.Second)
	samples := f.capture.snapshot()[f.collections[0].TargetID]
	for _, metric := range required {
		if !slices.Contains(samples, metric) {
			t.Fatalf("%s omitted required native metric %s: %v", key, metric, samples)
		}
	}
	if dir := os.Getenv("ARGUS_DATABASE_METRIC_EVIDENCE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		f.save(filepath.Join(dir, key+".json"), 1)
	}
	f.must("", "stop", "--time", "1", first)
	f.capture.reset()
	f.credentials[f.collections[0].ID]["password"] = "wrong-monitor-password"
	second := f.run(2)
	time.Sleep(12 * time.Second)
	if len(f.capture.snapshot()) != 0 {
		t.Fatal("failed database authentication produced target metrics")
	}
	f.must("", "stop", "--time", "1", second)
	f.capture.reset()
	f.credentials[f.collections[0].ID]["password"] = databaseTestPassword
	third := f.run(3)
	f.wait(third, 1)
	f.must("", "stop", "--time", "1", third)
	f.capture.reset()
	f.collections[0].Enabled = false
	f.collections[0].Revision++
	fourth := f.run(4)
	time.Sleep(3 * time.Second)
	if len(f.capture.snapshot()) != 0 || f.must("", "inspect", "--format", "{{.State.Running}}", fourth) != "true" {
		t.Fatal("disabled database still emitted data or stopped control runtime")
	}
	t.Logf("%s: actual native samples, read-only account, bad authentication, recovery and disabled control runtime verified", key)
}
