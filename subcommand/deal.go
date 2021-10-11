package subcommand

import (
	"fmt"
	"go-swan-client/common/client"
	"go-swan-client/config"
	"go-swan-client/logs"
	"go-swan-client/model"
	"math"
	"path/filepath"
	"strconv"
	"strings"
)

const DURATION = "1051200"
const EPOCH_PER_HOUR = 120

func SendDeals(minerFid string, outputDir *string, metadataJsonPath string) bool {
	if outputDir == nil {
		outDir := config.GetConfig().Sender.OutputDir
		outputDir = &outDir
	}
	metadataJsonFilename := filepath.Base(metadataJsonPath)
	taskName := strings.TrimSuffix(metadataJsonFilename, JSON_FILE_NAME_BY_TASK_SUFFIX)
	carFiles := ReadCarFilesFromJsonFileByFullPath(metadataJsonPath)
	if carFiles == nil {
		logs.GetLogger().Error("Failed to read car files from json.")
		return false
	}

	result := SendDeals2Miner(nil, taskName, minerFid, *outputDir, carFiles)

	return result
}

func GetDealConfig(minerFid string) *model.DealConfig {
	dealConfig := model.DealConfig{
		MinerFid:           minerFid,
		SenderWallet:       config.GetConfig().Sender.Wallet,
		VerifiedDeal:       config.GetConfig().Sender.VerifiedDeal,
		FastRetrieval:      config.GetConfig().Sender.FastRetrieval,
		EpochIntervalHours: config.GetConfig().Sender.StartEpochHours,
		SkipConfirmation:   config.GetConfig().Sender.SkipConfirmation,
		StartEpochHours:    config.GetConfig().Sender.StartEpochHours,
	}
	maxPriceStr := config.GetConfig().Sender.MaxPrice
	maxPrice, err := strconv.ParseFloat(maxPriceStr, 64)
	if err != nil {
		logs.GetLogger().Error("Failed to convert maxPrice to float, MaxPrice:", maxPriceStr)
		return nil
	}
	dealConfig.MaxPrice = maxPrice

	return &dealConfig
}

func CheckDealConfig(dealConfig model.DealConfig) bool {
	minerPrice, minerVerifiedPrice, _, _ := client.LotusGetMinerConfig(dealConfig.MinerFid)

	if dealConfig.VerifiedDeal {
		if minerVerifiedPrice == nil {
			return false
		}
		dealConfig.MinerPrice = *minerVerifiedPrice
	} else {
		if minerPrice == nil {
			return false
		}
		dealConfig.MinerPrice = *minerPrice
	}

	msg := fmt.Sprintf("Miner price is:%f, VerifiedDeal:%t", dealConfig.MinerPrice, dealConfig.VerifiedDeal)
	logs.GetLogger().Info(msg)
	if dealConfig.MaxPrice < dealConfig.MinerPrice {
		msg := fmt.Sprintf("miner %s price %f higher than max price %f", dealConfig.MinerFid, dealConfig.MinerPrice, dealConfig.MaxPrice)
		logs.GetLogger().Error(msg)
		return false
	}

	logs.GetLogger().Info("Deal check passed.")

	return true
}

func SendDeals2Miner(dealConfig *model.DealConfig, taskName string, minerFid string, outputDir string, carFiles []*model.FileDesc) bool {
	if dealConfig == nil {
		dealConfig := GetDealConfig(minerFid)
		if dealConfig == nil {
			logs.GetLogger().Error("Failed to get deal config.")
			return false
		}
	}

	result := CheckDealConfig(*dealConfig)
	if !result {
		logs.GetLogger().Error("Failed to pass deal config check.")
		return false
	}

	for _, carFile := range carFiles {
		if carFile.CarFileSize <= 0 {
			msg := fmt.Sprintf("File %s is too small", carFile.CarFilePath)
			logs.GetLogger().Error(msg)
			continue
		}
		pieceSize, sectorSize := CalculatePieceSize(carFile.CarFileSize)
		cost := CalculateRealCost(sectorSize, dealConfig.MinerPrice)
		dealCid, startEpoch := client.LotusProposeOfflineDeal(dealConfig.MinerPrice, cost, pieceSize, carFile.DataCid, carFile.PieceCid, *dealConfig)
		carFile.MinerFid = &minerFid
		carFile.DealCid = *dealCid
		carFile.StartEpoch = strconv.Itoa(*startEpoch)
	}

	jsonFileName := taskName + JSON_FILE_NAME_BY_DEAL_SUFFIX
	csvFileName := taskName + CSV_FILE_NAME_BY_DEAL_SUFFIX
	WriteCarFilesToFiles(carFiles, outputDir, jsonFileName, csvFileName)
	CreateCsv4TaskDeal(taskName, carFiles, &minerFid, outputDir)

	return true
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

func CalculateRealCost(sectorSizeBytes float64, pricePerGiB float64) float64 {
	var bytesPerGiB float64 = 1024 * 1024 * 1024
	sectorSizeGiB := float64(sectorSizeBytes) / bytesPerGiB

	realCost := sectorSizeGiB * pricePerGiB
	return realCost
}
