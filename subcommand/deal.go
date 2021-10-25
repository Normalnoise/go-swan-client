package subcommand

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/DoraNebula/go-swan-client/common/client"
	"github.com/DoraNebula/go-swan-client/common/constants"
	"github.com/DoraNebula/go-swan-client/logs"
	"github.com/DoraNebula/go-swan-client/model"

	"github.com/shopspring/decimal"
)

func SendDeals(confDeal *model.ConfDeal) error {
	logs.GetLogger().Info(confDeal.OutputDir)
	err := CreateOutputDir(confDeal.OutputDir)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	metadataJsonFilename := filepath.Base(*confDeal.MetadataJsonPath)
	taskName := strings.TrimSuffix(metadataJsonFilename, constants.JSON_FILE_NAME_BY_TASK)
	carFiles := ReadCarFilesFromJsonFileByFullPath(*confDeal.MetadataJsonPath)
	if len(carFiles) == 0 {
		err := fmt.Errorf("no car files read from:%s", *confDeal.MetadataJsonPath)
		logs.GetLogger().Error(err)
		return err
	}

	swanClient, err := client.SwanGetClient(confDeal.SwanApiUrl, confDeal.SwanApiKey, confDeal.SwanAccessToken)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}
	task, err := swanClient.GetOfflineDealsByTaskUuid(*carFiles[0].Uuid)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	if task.Data.Task.BidMode == nil && *task.Data.Task.BidMode != constants.TASK_BID_MODE_MANUAL {
		err := fmt.Errorf("auto_bid mode for task:%s is not manual, please check", task.Data.Task.TaskName)
		logs.GetLogger().Error(err)
		return err
	}

	if task.Data.Task.IsPublic == nil && *task.Data.Task.IsPublic != constants.TASK_IS_PUBLIC {
		err := fmt.Errorf("task:%s is not in public mode,please check", task.Data.Task.TaskName)
		logs.GetLogger().Error(err)
		return err
	}

	csvFilepath, err := SendDeals2Miner(confDeal, taskName, *confDeal.MinerFid, confDeal.OutputDir, carFiles)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	err = swanClient.SwanUpdateTaskByUuid(*carFiles[0].Uuid, *confDeal.MinerFid, *csvFilepath)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	return nil
}

func SendDeals2Miner(confDeal *model.ConfDeal, taskName string, minerFid string, outputDir string, carFiles []*model.FileDesc) (*string, error) {
	err := CheckDealConfig(confDeal)
	if err != nil {
		err := errors.New("failed to pass deal config check")
		logs.GetLogger().Error(err)
		return nil, err
	}

	for _, carFile := range carFiles {
		if carFile.CarFileSize <= 0 {
			logs.GetLogger().Error("File:" + carFile.CarFilePath + " %s is too small")
			continue
		}
		pieceSize, sectorSize := CalculatePieceSize(carFile.CarFileSize)
		logs.GetLogger().Info("dealConfig.MinerPrice:", confDeal.MinerPrice)
		cost := CalculateRealCost(sectorSize, confDeal.MinerPrice)
		dealCid, startEpoch, err := client.LotusProposeOfflineDeal(*carFile, cost, pieceSize, *confDeal, 0)
		//dealCid, err := client.LotusClientStartDeal(*carFile, cost, pieceSize, *dealConfig)
		if err != nil {
			logs.GetLogger().Error(err)
			continue
		}
		if dealCid == nil {
			continue
		}
		carFile.MinerFid = confDeal.MinerFid
		carFile.DealCid = dealCid
		carFile.StartEpoch = *startEpoch

		logs.GetLogger().Info("Cid:", carFile.DealCid, " start epoch:", carFile.StartEpoch)
	}

	jsonFileName := taskName + constants.JSON_FILE_NAME_BY_DEAL
	csvFileName := taskName + constants.CSV_FILE_NAME_BY_DEAL
	err = WriteCarFilesToFiles(carFiles, outputDir, jsonFileName, csvFileName)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, err
	}

	csvFilename := taskName + "-deals.csv"
	csvFilepath, err := CreateCsv4TaskDeal(carFiles, outputDir, csvFilename)

	return &csvFilepath, err
}

// https://docs.filecoin.io/store/lotus/very-large-files/#maximizing-storage-per-sector
func CalculatePieceSize(fileSize int64) (int64, float64) {
	exp := math.Ceil(math.Log2(float64(fileSize)))
	sectorSize2Check := math.Pow(2, exp)
	pieceSize2Check := int64(sectorSize2Check * 254 / 256)
	if fileSize <= pieceSize2Check {
		return pieceSize2Check, sectorSize2Check
	}

	exp = exp + 1
	realSectorSize := math.Pow(2, exp)
	realPieceSize := int64(realSectorSize * 254 / 256)
	return realPieceSize, realSectorSize
}

func CalculateRealCost(sectorSizeBytes float64, pricePerGiB decimal.Decimal) decimal.Decimal {
	logs.GetLogger().Info("sectorSizeBytes:", sectorSizeBytes, " pricePerGiB:", pricePerGiB)
	bytesPerGiB := decimal.NewFromInt(1024 * 1024 * 1024)
	sectorSizeGiB := decimal.NewFromFloat(sectorSizeBytes).Div(bytesPerGiB)
	realCost := sectorSizeGiB.Mul(pricePerGiB)
	logs.GetLogger().Info("realCost:", realCost)
	return realCost
}
