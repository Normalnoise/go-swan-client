package command

import (
	"fmt"

	"github.com/filswan/go-swan-lib/client/ipfs"
	libmodel "github.com/filswan/go-swan-lib/model"

	"github.com/filswan/go-swan-client/config"

	libconstants "github.com/filswan/go-swan-lib/constants"
	"github.com/filswan/go-swan-lib/logs"
	"github.com/filswan/go-swan-lib/utils"
)

type CmdUpload struct {
	StorageServerType           string //required
	IpfsServerDownloadUrlPrefix string //required only when upload to ipfs server
	IpfsServerUploadUrlPrefix   string //required only when upload to ipfs server
	OutputDir                   string //invalid
	InputDir                    string //required
}

func GetCmdUpload(inputDir string) *CmdUpload {
	cmdUpload := &CmdUpload{
		StorageServerType:           config.GetConfig().Main.StorageServerType,
		IpfsServerDownloadUrlPrefix: config.GetConfig().IpfsServer.DownloadUrlPrefix,
		IpfsServerUploadUrlPrefix:   config.GetConfig().IpfsServer.UploadUrlPrefix,
		OutputDir:                   inputDir,
		InputDir:                    inputDir,
	}

	return cmdUpload
}

func UploadCarFilesByConfig(inputDir string) ([]*libmodel.FileDesc, error) {
	cmdUpload := GetCmdUpload(inputDir)

	fileDescs, err := cmdUpload.UploadCarFiles()
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, err
	}

	return fileDescs, nil
}

func (cmdUpload *CmdUpload) UploadCarFiles() ([]*libmodel.FileDesc, error) {
	err := utils.CheckDirExists(cmdUpload.InputDir, DIR_NAME_INPUT)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, err
	}

	if cmdUpload.StorageServerType == libconstants.STORAGE_SERVER_TYPE_WEB_SERVER {
		logs.GetLogger().Info("Please upload car files to web server manually.")
		return nil, nil
	}

	fileDescs, err := ReadFileDescsFromJsonFile(cmdUpload.InputDir, JSON_FILE_NAME_CAR_UPLOAD)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, err
	}

	if fileDescs == nil {
		err := fmt.Errorf("failed to read:%s", cmdUpload.InputDir)
		logs.GetLogger().Error(err)
		return nil, err
	}

	uploadUrl := utils.UrlJoin(cmdUpload.IpfsServerUploadUrlPrefix, "api/v0/add?stream-channels=true&pin=true")
	for _, fileDesc := range fileDescs {
		logs.GetLogger().Info("Uploading car file:", fileDesc.CarFilePath, " to:", uploadUrl)
		carFileHash, err := ipfs.IpfsUploadFileByWebApi(uploadUrl, fileDesc.CarFilePath)
		if err != nil {
			logs.GetLogger().Error(err)
			return nil, err
		}

		carFileUrl := utils.UrlJoin(cmdUpload.IpfsServerDownloadUrlPrefix, "ipfs", *carFileHash)
		fileDesc.CarFileUrl = carFileUrl
		logs.GetLogger().Info("Car file: ", fileDesc.CarFileName, " uploaded to: ", fileDesc.CarFileUrl)
	}

	logs.GetLogger().Info(len(fileDescs), " car files have been uploaded to:", uploadUrl)

	_, err = WriteFileDescsToJsonFile(fileDescs, cmdUpload.InputDir, JSON_FILE_NAME_CAR_UPLOAD)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, err
	}

	logs.GetLogger().Info("Please create a task for your car file(s)")

	return fileDescs, nil
}
