package model

import (
	"done-hub/common/config"
	"done-hub/common/logger"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func removeKeyIndexMigration() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202405152141",
		Migrate: func(tx *gorm.DB) error {
			dialect := tx.Dialector.Name()
			if dialect == "sqlite" {
				return nil
			}

			if !tx.Migrator().HasIndex(&Channel{}, "idx_channels_key") {
				return nil
			}

			err := tx.Migrator().DropIndex(&Channel{}, "idx_channels_key")
			if err != nil {
				logger.SysLog("remove idx_channels_key  Failure: " + err.Error())
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

func changeTokenKeyColumnType() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202411300001",
		Migrate: func(tx *gorm.DB) error {
			// 如果表不存在，说明是新数据库，直接跳过
			if !tx.Migrator().HasTable("tokens") {
				return nil
			}

			dialect := tx.Dialector.Name()
			var err error

			switch dialect {
			case "mysql":
				err = tx.Exec("ALTER TABLE tokens MODIFY COLUMN `key` varchar(59)").Error
			case "postgres":
				err = tx.Exec("ALTER TABLE tokens ALTER COLUMN key TYPE varchar(59)").Error
			case "sqlite":
				return nil
			}

			if err != nil {
				logger.SysLog("修改 tokens.key 字段类型失败: " + err.Error())
				return err
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if !tx.Migrator().HasTable("tokens") {
				return nil
			}

			dialect := tx.Dialector.Name()
			var err error

			switch dialect {
			case "mysql":
				err = tx.Exec("ALTER TABLE tokens MODIFY COLUMN `key` char(48)").Error
			case "postgres":
				err = tx.Exec("ALTER TABLE tokens ALTER COLUMN key TYPE char(48)").Error
			}
			return err
		},
	}
}

func migrationBefore(db *gorm.DB) error {
	// 从库不执行
	if !config.IsMasterNode {
		logger.SysLog("从库不执行迁移前操作")
		return nil
	}

	// 如果是第一次运行 直接跳过
	if !db.Migrator().HasTable("channels") {
		return nil
	}

	m := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		removeKeyIndexMigration(),
		changeTokenKeyColumnType(),
	})
	return m.Migrate()
}

func addStatistics() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202408100001",
		Migrate: func(tx *gorm.DB) error {
			go UpdateStatistics(StatisticsUpdateTypeALL)
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

func changeChannelApiVersion() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202408190001",
		Migrate: func(tx *gorm.DB) error {
			plugin := `{"customize": {"1": "{version}/chat/completions", "2": "{version}/completions", "3": "{version}/embeddings", "4": "{version}/moderations", "5": "{version}/images/generations", "6": "{version}/images/edits", "7": "{version}/images/variations", "9": "{version}/audio/speech", "10": "{version}/audio/transcriptions", "11": "{version}/audio/translations"}}`

			// 查询 channel 表中的type 为 8，且 other = disable 的数据,直接更新
			var jsonMap map[string]map[string]interface{}
			err := json.Unmarshal([]byte(strings.Replace(plugin, "{version}", "", -1)), &jsonMap)
			if err != nil {
				logger.SysLog("changeChannelApiVersion Failure: " + err.Error())
				return err
			}
			disableApi := map[string]interface{}{
				"other":  "",
				"plugin": datatypes.NewJSONType(jsonMap),
			}

			err = tx.Model(&Channel{}).Where("type = ? AND other = ?", 8, "disable").Updates(disableApi).Error
			if err != nil {
				logger.SysLog("changeChannelApiVersion Failure: " + err.Error())
				return err
			}

			// 查询 channel 表中的type 为 8，且 other != disable 并且不为空 的数据,直接更新
			var channels []*Channel
			err = tx.Model(&Channel{}).Where("type = ? AND other != ? AND other != ?", 8, "disable", "").Find(&channels).Error
			if err != nil {
				logger.SysLog("changeChannelApiVersion Failure: " + err.Error())
				return err
			}

			for _, channel := range channels {
				var jsonMap map[string]map[string]interface{}
				err := json.Unmarshal([]byte(strings.Replace(plugin, "{version}", "/"+channel.Other, -1)), &jsonMap)
				if err != nil {
					logger.SysLog("changeChannelApiVersion Failure: " + err.Error())
					return err
				}
				changeApi := map[string]interface{}{
					"other":  "",
					"plugin": datatypes.NewJSONType(jsonMap),
				}
				err = tx.Model(&Channel{}).Where("id = ?", channel.Id).Updates(changeApi).Error
				if err != nil {
					logger.SysLog("changeChannelApiVersion Failure: " + err.Error())
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Rollback().Error
		},
	}
}

func initUserGroup() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202410300001",
		Migrate: func(tx *gorm.DB) error {
			userGroups := map[string]*UserGroup{
				"default": {
					Symbol: "default",
					Name:   "默认分组",
					Ratio:  1,
					Public: true,
				},
				"vip": {
					Symbol: "vip",
					Name:   "vip分组",
					Ratio:  1,
					Public: false,
				},
				"svip": {
					Symbol: "svip",
					Name:   "svip分组",
					Ratio:  1,
					Public: false,
				},
			}
			option, err := GetOption("GroupRatio")
			if err == nil && option.Value != "" {
				oldGroup := make(map[string]float64)
				err = json.Unmarshal([]byte(option.Value), &oldGroup)
				if err != nil {
					return err
				}

				for k, v := range oldGroup {
					isPublic := false
					if k == "default" {
						isPublic = true
					}
					userGroups[k] = &UserGroup{
						Symbol: k,
						Name:   k,
						Ratio:  v,
						Public: isPublic,
					}
				}
			}

			for k, v := range userGroups {
				err := tx.Where("symbol = ?", k).FirstOrCreate(v).Error
				if err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Rollback().Error
		},
	}
}

func addOldTokenMaxId() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202411300002",
		Migrate: func(tx *gorm.DB) error {
			var token Token
			tx.Last(&token)
			tokenMaxId := token.Id
			option := Option{
				Key: "OldTokenMaxId",
			}

			DB.FirstOrCreate(&option, Option{Key: "OldTokenMaxId"})
			option.Value = strconv.Itoa(tokenMaxId)
			return DB.Save(&option).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Rollback().Error
		},
	}
}

func addExtraRatios() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202504300001",
		Migrate: func(tx *gorm.DB) error {
			extraTokenPriceJson := ""
			extraRatios := make(map[string]map[string]float64)
			// 先查询数据库中是否存在extra_ratios
			option, err := GetOption("ExtraTokenPriceJson")
			if err == nil {
				extraTokenPriceJson = option.Value

			} else {
				extraTokenPriceJson = GetDefaultExtraRatio()
			}

			err = json.Unmarshal([]byte(extraTokenPriceJson), &extraRatios)
			if err != nil {
				return err
			}

			if len(extraRatios) == 0 {
				return nil
			}

			models := make([]string, 0)
			for model := range extraRatios {
				models = append(models, model)
			}

			// 查询数据库中是否存在
			var prices []*Price
			err = tx.Where("model IN (?)", models).Find(&prices).Error
			if err != nil {
				return err
			}

			for _, price := range prices {
				extraRatios := extraRatios[price.Model]
				jsonData := datatypes.NewJSONType(extraRatios)
				price.ExtraRatios = &jsonData
				err = tx.Model(&Price{}).Where("model = ?", price.Model).Updates(map[string]interface{}{
					"extra_ratios": jsonData,
				}).Error
				if err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Rollback().Error
		},
	}
}

// addCachedWrite1hRatio 为已存在 ExtraRatios 的模型补全 1h 缓存写入倍率。
// 仅作 UI 显示对齐：计费侧 defaultExtraPrice[cached_write_1h_tokens]=2.0 已能兜底，
// 此处把数字落到 ExtraRatios 里是为了让前端表格显示明确数字而不是空白/默认。
// 按 key 触发而非 channel_type，确保 Bedrock / Vertex 上跑的 Claude 渠道也覆盖到；
// 仅在已配置 cached_write_tokens 且未配置 cached_write_1h_tokens 时按官方倍率 2.0 写入，
// 不覆盖用户自定义值。
// 副作用：非 Claude 模型若手配 cached_write_tokens 也会被塞入 1h，但其请求路径不上报
// cached_write_1h_tokens，不会被计费，无害。
func addCachedWrite1hRatio() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202606100001",
		Migrate: func(tx *gorm.DB) error {
			var prices []*Price
			if err := tx.Find(&prices).Error; err != nil {
				return err
			}

			for _, price := range prices {
				if price.ExtraRatios == nil {
					continue
				}
				ratios := price.ExtraRatios.Data()
				if _, ok := ratios[config.UsageExtraCachedWrite]; !ok {
					continue
				}
				if _, ok := ratios[config.UsageExtraCachedWrite1h]; ok {
					continue
				}
				ratios[config.UsageExtraCachedWrite1h] = 2
				jsonData := datatypes.NewJSONType(ratios)
				if err := tx.Model(&Price{}).Where("model = ?", price.Model).Updates(map[string]interface{}{
					"extra_ratios": jsonData,
				}).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Rollback().Error
		},
	}
}

func migrateTokenLimitsStructure() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202510160002",
		Migrate: func(tx *gorm.DB) error {
			// 直接查询原始JSON字符串，避免GORM自动转换
			type TokenRaw struct {
				Id      int    `gorm:"column:id"`
				Name    string `gorm:"column:name"`
				Setting string `gorm:"column:setting;type:json"`
			}

			var tokens []TokenRaw
			err := tx.Table("tokens").Select("id, name, setting").Find(&tokens).Error
			if err != nil {
				logger.SysLog("查询token列表失败: " + err.Error())
				return err
			}

			// 遍历每个 token，转换 limits 结构
			for _, token := range tokens {
				// 解析为 map 以便灵活处理
				var settingMap map[string]interface{}
				err = json.Unmarshal([]byte(token.Setting), &settingMap)
				if err != nil || settingMap == nil {
					// 如果解析失败或为空，跳过
					continue
				}

				// 检查是否有 limits 字段
				limitsRaw, exists := settingMap["limits"]
				if !exists || limitsRaw == nil {
					continue
				}

				// 将 limits 转换为 map
				limitsMap, ok := limitsRaw.(map[string]interface{})
				if !ok {
					continue
				}

				// 检查是否已经是新结构（包含 limit_model_setting）
				if _, hasNew := limitsMap["limit_model_setting"]; hasNew {
					// 已经是新结构，跳过
					continue
				}

				// 检查是否是旧结构（包含 enabled 或 models 字段，说明是直接在 limits 下的旧结构）
				_, hasEnabled := limitsMap["enabled"]
				_, hasModels := limitsMap["models"]
				if !hasEnabled && !hasModels {
					// 既没有 enabled 也没有 models，说明不是旧结构，跳过
					continue
				}

				// 转换为新结构：将旧的 limits 内容移到 limit_model_setting 下
				newLimits := map[string]interface{}{
					"limit_model_setting": limitsMap,
					"limits_ip_setting":   LimitsIPSetting{},
				}

				// 更新 settingMap
				settingMap["limits"] = newLimits

				// 序列化回 JSON
				newSettingBytes, err := json.Marshal(settingMap)
				if err != nil {
					logger.SysLog("token setting序列化失败: " + err.Error())
					continue
				}

				// 更新数据库
				err = tx.Model(&Token{}).Where("id = ?", token.Id).Update("setting", datatypes.JSON(newSettingBytes)).Error
				if err != nil {
					logger.SysLog("更新token setting失败: " + err.Error())
					continue
				}
			}

			logger.SysLog("Token表setting字段limits结构升级完成")
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 回滚：将新结构转回旧结构
			var tokens []Token
			err := tx.Find(&tokens).Error
			if err != nil {
				return err
			}

			for _, token := range tokens {
				settingBytes, err := token.Setting.MarshalJSON()
				if err != nil {
					continue
				}

				var settingMap map[string]interface{}
				err = json.Unmarshal(settingBytes, &settingMap)
				if err != nil || settingMap == nil {
					continue
				}

				limitsRaw, exists := settingMap["limits"]
				if !exists || limitsRaw == nil {
					continue
				}

				limitsMap, ok := limitsRaw.(map[string]interface{})
				if !ok {
					continue
				}

				// 检查是否有 limit_model_setting
				modelSettingRaw, hasModelSetting := limitsMap["limit_model_setting"]
				if !hasModelSetting {
					continue
				}

				// 将 limit_model_setting 的内容提升到 limits 层级
				settingMap["limits"] = modelSettingRaw

				newSettingBytes, err := json.Marshal(settingMap)
				if err != nil {
					continue
				}

				tx.Model(&Token{}).Where("id = ?", token.Id).Update("setting", datatypes.JSON(newSettingBytes))
			}

			return nil
		},
	}
}

const fixModelQuotaByPriceProcedureSQL = `
CREATE PROCEDURE fix_model_quota_by_price(
  IN p_model VARCHAR(100),
  IN p_start_ts BIGINT,
  IN p_end_ts BIGINT,
  IN p_apply TINYINT
)
BEGIN
  DECLARE v_input DECIMAL(20,8);
  DECLARE v_output DECIMAL(20,8);
  DECLARE v_price_type VARCHAR(32);
  DECLARE v_price_count INT DEFAULT 0;
  DECLARE v_table_exists INT DEFAULT 0;
  DECLARE v_backup_table VARCHAR(64);
  DECLARE v_msg VARCHAR(255);

  DECLARE EXIT HANDLER FOR SQLEXCEPTION
  BEGIN
    ROLLBACK;
    RESIGNAL;
  END;

  IF p_model IS NULL OR TRIM(p_model) = '' THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'model is required';
  END IF;

  IF p_start_ts IS NULL OR p_end_ts IS NULL OR p_start_ts <= 0 OR p_start_ts >= p_end_ts THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'invalid time range';
  END IF;

  IF COALESCE(p_apply, -1) NOT IN (0, 1) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'p_apply must be 0 or 1';
  END IF;

  SELECT COUNT(*) INTO v_price_count
  FROM prices
  WHERE model = p_model;

  IF v_price_count = 0 THEN
    SET v_msg = CONCAT('price not found for model: ', p_model);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = v_msg;
  END IF;

  SELECT ` + "`type`" + `, ` + "`input`" + `, ` + "`output`" + `
  INTO v_price_type, v_input, v_output
  FROM prices
  WHERE model = p_model
  LIMIT 1;

  IF COALESCE(v_price_type, '') <> 'tokens' THEN
    SET v_msg = CONCAT('only tokens price type is supported, got: ', COALESCE(v_price_type, ''));
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = v_msg;
  END IF;

  SET v_backup_table = CONCAT(
    'logs_fix_quota_',
    LEFT(MD5(p_model), 8),
    '_',
    p_start_ts,
    '_',
    p_end_ts
  );

  IF p_apply = 0 THEN
    SELECT
      p_model AS model,
      FROM_UNIXTIME(p_start_ts) AS start_time,
      FROM_UNIXTIME(p_end_ts) AS end_time_exclusive,
      v_input AS input_ratio,
      v_output AS output_ratio,
      COUNT(*) AS rows_to_fix,
      COALESCE(SUM(quota), 0) AS old_quota,
      COALESCE(SUM(new_quota), 0) AS new_quota,
      COALESCE(SUM(quota - new_quota), 0) AS reduced_quota
    FROM (
      SELECT
        quota,
        CEIL(
          (prompt_tokens * v_input + completion_tokens * v_output)
          * COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(metadata, '$.group_ratio')) AS DECIMAL(20,8)), 1)
        ) AS new_quota
      FROM logs
      WHERE type = 2
        AND model_name = p_model
        AND created_at >= p_start_ts
        AND created_at < p_end_ts
    ) x;
  ELSE
    SELECT COUNT(*) INTO v_table_exists
    FROM information_schema.tables
    WHERE table_schema = DATABASE()
      AND table_name = v_backup_table;

    IF v_table_exists > 0 THEN
      SET v_msg = CONCAT('backup table already exists: ', v_backup_table);
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = v_msg;
    END IF;

    SET @fix_input = v_input;
    SET @fix_output = v_output;
    SET @fix_model = p_model;
    SET @fix_start = p_start_ts;
    SET @fix_end = p_end_ts;

    SET @sql = CONCAT(
      'CREATE TABLE ', v_backup_table, ' AS ',
      'SELECT ',
      'id, user_id, username, token_name, model_name, created_at, ',
      'prompt_tokens, completion_tokens, ',
      'quota AS old_quota, ',
      'CEIL((prompt_tokens * ? + completion_tokens * ?) * ',
      'COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(metadata, ''$.group_ratio'')) AS DECIMAL(20,8)), 1)) AS new_quota, ',
      'COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(metadata, ''$.group_ratio'')) AS DECIMAL(20,8)), 1) AS group_ratio, ',
      'NOW() AS backup_created_at ',
      'FROM logs ',
      'WHERE type = 2 ',
      'AND model_name = ? ',
      'AND created_at >= ? ',
      'AND created_at < ?'
    );

    PREPARE stmt FROM @sql;
    EXECUTE stmt USING @fix_input, @fix_output, @fix_model, @fix_start, @fix_end;
    DEALLOCATE PREPARE stmt;

    SET @sql = CONCAT('ALTER TABLE ', v_backup_table, ' ADD PRIMARY KEY (id)');
    PREPARE stmt FROM @sql;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;

    START TRANSACTION;

    SET @sql = CONCAT(
      'UPDATE logs l ',
      'JOIN ', v_backup_table, ' b ON b.id = l.id ',
      'SET l.quota = b.new_quota'
    );

    PREPARE stmt FROM @sql;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;

    COMMIT;

    SET @sql = CONCAT(
      'SELECT ',
      QUOTE(v_backup_table),
      ' AS backup_table, ',
      'COUNT(*) AS rows_fixed, ',
      'COALESCE(SUM(new_quota), 0) AS fixed_quota, ',
      'COALESCE(SUM(old_quota), 0) AS old_quota, ',
      'COALESCE(SUM(old_quota - new_quota), 0) AS reduced_quota ',
      'FROM ', v_backup_table
    );

    PREPARE stmt FROM @sql;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;
  END IF;
END
`

const fixModelQuotaByPriceDayProcedureSQL = `
CREATE PROCEDURE fix_model_quota_by_price_day(
  IN p_model VARCHAR(100),
  IN p_start_day VARCHAR(16),
  IN p_end_day VARCHAR(16),
  IN p_apply TINYINT
)
BEGIN
  DECLARE v_start_dt DATETIME;
  DECLARE v_end_dt DATETIME;
  DECLARE v_start_ts BIGINT;
  DECLARE v_end_ts BIGINT;

  SET v_start_dt = COALESCE(
    STR_TO_DATE(p_start_day, '%Y.%m.%d'),
    STR_TO_DATE(p_start_day, '%Y-%m-%d')
  );
  SET v_end_dt = COALESCE(
    STR_TO_DATE(p_end_day, '%Y.%m.%d'),
    STR_TO_DATE(p_end_day, '%Y-%m-%d')
  );

  IF v_start_dt IS NULL OR v_end_dt IS NULL OR v_start_dt >= v_end_dt THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'invalid date range, expected format: 2026.04.01';
  END IF;

  SET v_start_ts = TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', v_start_dt) - 28800;
  SET v_end_ts = TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', v_end_dt) - 28800;

  CALL fix_model_quota_by_price(p_model, v_start_ts, v_end_ts, p_apply);
END
`

func installQuotaFixProcedures() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202604290001",
		Migrate: func(tx *gorm.DB) error {
			if tx.Dialector.Name() != "mysql" {
				return nil
			}

			// CREATE PROCEDURE is not supported via MySQL's prepared statement
			// protocol (Error 1295). Even with gorm's PrepareStmt:false session
			// option, statements may still be prepared. Use the underlying
			// *sql.DB.Exec directly to force the text protocol.
			sqlDB, err := tx.DB()
			if err != nil {
				return err
			}
			statements := []string{
				"DROP PROCEDURE IF EXISTS fix_model_quota_by_price",
				fixModelQuotaByPriceProcedureSQL,
				"DROP PROCEDURE IF EXISTS fix_model_quota_by_price_day",
				fixModelQuotaByPriceDayProcedureSQL,
			}
			for _, statement := range statements {
				if _, err := sqlDB.Exec(statement); err != nil {
					logger.SysLog("安装模型历史计费修复存储过程失败: " + err.Error())
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if tx.Dialector.Name() != "mysql" {
				return nil
			}
			sqlDB, err := tx.DB()
			if err != nil {
				return err
			}
			if _, err := sqlDB.Exec("DROP PROCEDURE IF EXISTS fix_model_quota_by_price_day"); err != nil {
				return err
			}
			_, err = sqlDB.Exec("DROP PROCEDURE IF EXISTS fix_model_quota_by_price")
			return err
		},
	}
}

func migrationAfter(db *gorm.DB) error {
	// 从库不执行
	if !config.IsMasterNode {
		logger.SysLog("从库不执行迁移后操作")
		return nil
	}
	m := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		addStatistics(),
		changeChannelApiVersion(),
		initUserGroup(),
		addOldTokenMaxId(),
		addExtraRatios(),
		migrateTokenLimitsStructure(),
		installQuotaFixProcedures(),
	})
	return m.Migrate()
}
