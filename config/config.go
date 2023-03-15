package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

var (
	// 默认跟路径下有conf文件夹
	rootConfPath string
	// 默认配置文件的文件夹
	confDir = "conf"
)

// InitConfig 初始化配置文件
func InitConfig() {
	if err := readConfPath(); err != nil {
		panic(err)
	}
}

// GetConfPath 获取配置文件路径
func GetConfPath() string {
	return rootConfPath
}

func readConfPath() error {
	curDir, err := os.Getwd()
	if err != nil {
		return err
	}

	for {
		if curDir == "/" {
			return fmt.Errorf("%s dir not found", confDir)
		}

		dirs, err := ioutil.ReadDir(curDir)
		if err != nil {
			return err
		}

		for _, dir := range dirs {
			if !dir.IsDir() {
				continue
			}

			if dir.Name() == confDir {
				rootConfPath = path.Join(curDir, confDir)
				return nil
			}
		}

		curDir = filepath.Dir(curDir)
	}
}
