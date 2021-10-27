package subcommand

import (
	"fmt"
	"time"

	"github.com/filswan/go-swan-client/model"
	"github.com/filswan/go-swan-lib/logs"

	"github.com/filswan/go-swan-lib/client"
	"github.com/filswan/go-swan-lib/constants"
	libmodel "github.com/filswan/go-swan-lib/model"
	"github.com/filswan/go-swan-lib/utils"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func CreateTask(confTask *model.ConfTask, confDeal *model.ConfDeal) (*string, []*libmodel.FileDesc, error) {
	err := CheckInputDir(confTask.InputDir)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, nil, err
	}

	err = CreateOutputDir(confTask.OutputDir)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, nil, err
	}

	logs.GetLogger().Info("you output dir: ", confTask.OutputDir)

	if !confTask.PublicDeal && (confTask.MinerFid == nil || len(*confTask.MinerFid) == 0) {
		err := fmt.Errorf("please provide -miner for private deal")
		logs.GetLogger().Error(err)
		return nil, nil, err
	}
	if confTask.BidMode == constants.TASK_BID_MODE_AUTO && confTask.MinerFid != nil && len(*confTask.MinerFid) != 0 {
		logs.GetLogger().Warn("miner is unnecessary for aubo-bid task, it will be ignored")
	}

	if confTask.TaskName == nil || len(*confTask.TaskName) == 0 {
		nowStr := "task_" + time.Now().Format("2006-01-02_15:04:05")
		confTask.TaskName = &nowStr
	}

	maxPrice, err := decimal.NewFromString(confTask.MaxPrice)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, nil, err
	}
	//generateMd5 := config.GetConfig().Sender.GenerateMd5

	logs.GetLogger().Info("task settings:")
	logs.GetLogger().Info("public task: ", confTask.PublicDeal)
	logs.GetLogger().Info("verified deals: ", confTask.VerifiedDeal)
	logs.GetLogger().Info("connected to swan: ", !confTask.OfflineMode)
	logs.GetLogger().Info("fastRetrieval: ", confTask.FastRetrieval)

	carFiles := ReadCarFilesFromJsonFile(confTask.InputDir, constants.JSON_FILE_NAME_BY_UPLOAD)
	if carFiles == nil {
		err := fmt.Errorf("failed to read car files from :%s", confTask.InputDir)
		logs.GetLogger().Error(err)
		return nil, nil, err
	}

	isPublic := 0
	if confTask.PublicDeal {
		isPublic = 1
	}

	taskType := constants.TASK_TYPE_REGULAR
	if confTask.VerifiedDeal {
		taskType = constants.TASK_TYPE_VERIFIED
	}

	uuid := uuid.NewString()
	task := libmodel.Task{
		TaskName:          *confTask.TaskName,
		FastRetrievalBool: confTask.FastRetrieval,
		Type:              &taskType,
		IsPublic:          &isPublic,
		MaxPrice:          &maxPrice,
		BidMode:           &confTask.BidMode,
		ExpireDays:        &confTask.ExpireDays,
		MinerFid:          confTask.MinerFid,
		Uuid:              &uuid,
	}

	if confTask.Dataset != nil {
		task.CuratedDataset = *confTask.Dataset
	}

	if confTask.Description != nil {
		task.Description = *confTask.Description
	}

	for _, carFile := range carFiles {
		carFile.Uuid = task.Uuid
		carFile.MinerFid = task.MinerFid
		carFile.StartEpoch = confTask.StartEpoch

		if confTask.StorageServerType == constants.STORAGE_SERVER_TYPE_WEB_SERVER {
			carFileUrl := utils.UrlJoin(confTask.WebServerDownloadUrlPrefix, carFile.CarFileName)
			carFile.CarFileUrl = &carFileUrl
		}
	}

	if !confTask.PublicDeal {
		_, _, err := SendDeals2Miner(confDeal, *confTask.TaskName, confTask.OutputDir, carFiles)
		if err != nil {
			return nil, nil, err
		}
	}

	jsonFileName := *confTask.TaskName + constants.JSON_FILE_NAME_BY_TASK
	csvFileName := *confTask.TaskName + constants.CSV_FILE_NAME_BY_TASK
	err = WriteCarFilesToFiles(carFiles, confTask.OutputDir, jsonFileName, csvFileName)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, nil, err
	}

	err = SendTask2Swan(confTask, task, carFiles)
	if err != nil {
		logs.GetLogger().Error(err)
		return nil, nil, err
	}

	return &jsonFileName, carFiles, nil
}

func SendTask2Swan(confTask *model.ConfTask, task libmodel.Task, carFiles []*libmodel.FileDesc) error {
	csvFilename := task.TaskName + ".csv"
	csvFilePath, err := CreateCsv4TaskDeal(carFiles, confTask.OutputDir, csvFilename)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	if confTask.OfflineMode {
		logs.GetLogger().Info("Working in Offline Mode. You need to manually send out task on filwan.com.")
		return nil
	}

	logs.GetLogger().Info("Working in Online Mode. A swan task will be created on the filwan.com after process done. ")
	swanClient, err := client.SwanGetClient(confTask.SwanApiUrl, confTask.SwanApiKey, confTask.SwanAccessToken)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	swanCreateTaskResponse, err := swanClient.SwanCreateTask(task, csvFilePath)
	if err != nil {
		logs.GetLogger().Error(err)
		return err
	}

	if swanCreateTaskResponse.Status != "success" {
		err := fmt.Errorf("error, status%s, message:%s", swanCreateTaskResponse.Status, swanCreateTaskResponse.Message)
		logs.GetLogger().Info(err)
		return err
	}

	logs.GetLogger().Info("status:", swanCreateTaskResponse.Status, ", message:", swanCreateTaskResponse.Message)

	return nil
}
