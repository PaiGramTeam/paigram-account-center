package main

import (
	"fmt"
	"log"
	"os"

	"platform-mihomo-service/internal/testutil/integrationenv"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	repoRoot, err := findRepoRoot(wd)
	if err != nil {
		log.Fatal(err)
	}

	env, err := integrationenv.Load(repoRoot)
	if err != nil {
		log.Fatal(err)
	}

	if err := integrationenv.CheckMySQL(env); err != nil {
		log.Fatal(err)
	}
	if env.RedisAddr != "" {
		if err := integrationenv.CheckRedis(env, false); err != nil {
			log.Fatal(err)
		}
	}

	generatedDBName := integrationenv.UniqueDatabaseName(env.DatabaseBaseName, "doctor")
	generatedRedisPrefix := integrationenv.UniqueRedisPrefix(env.RedisPrefix, "doctor")
	for _, line := range env.Summary(generatedDBName, generatedRedisPrefix, os.Getenv("GOWORK"), false) {
		fmt.Println(line)
	}
}

func findRepoRoot(start string) (string, error) {
	return integrationenv.FindRepoRoot(start)
}
