// zebra-gw — ZebraGateway CLI 管理工具
//
// 用法:
//
//	zebra-gw route list
//	zebra-gw route add   -p /rbac -t http://192.168.30.198:8000 -r /api
//	zebra-gw route update <id> [flags]
//	zebra-gw route delete <id>
//	zebra-gw route enable  <id>
//	zebra-gw route disable <id>
//	zebra-gw whitelist list
//	zebra-gw whitelist add -m GET -p /health
//	zebra-gw whitelist delete <id>
package main

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/ZebraOps/ZebraGateway/config"
	"github.com/ZebraOps/ZebraGateway/internal/model"
	"github.com/ZebraOps/ZebraGateway/internal/store"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

var db *gorm.DB

func main() {
	root := &cobra.Command{
		Use:   "zebra-gw",
		Short: "ZebraGateway 管理工具",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Load()
			if cfg.DatabaseURL == "" {
				return fmt.Errorf("DatabaseURL 未配置，请检查 config/configs.yaml 或环境变量 ZEBRA_GW_APP_DATABASEURL")
			}
			var err error
			db, err = store.New(cfg.DatabaseURL)
			if err != nil {
				return fmt.Errorf("连接数据库失败: %w", err)
			}
			return nil
		},
	}

	root.AddCommand(buildRouteCmd(), buildWhitelistCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ─── route ─────────────────────────────────────────────────────────────────────

func buildRouteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "route",
		Short: "管理服务路由",
	}
	cmd.AddCommand(
		routeListCmd(),
		routeAddCmd(),
		routeUpdateCmd(),
		routeDeleteCmd(),
		routeEnableCmd(),
		routeDisableCmd(),
	)
	return cmd
}

func routeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出所有服务路由",
		RunE: func(cmd *cobra.Command, args []string) error {
			var routes []model.ServiceRoute
			if err := db.Find(&routes).Error; err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "ID\tPREFIX\tTARGET\tREWRITE\tENABLED\tDESCRIPTION")
			for _, r := range routes {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%v\t%s\n",
					r.ID, r.Prefix, r.Target, r.Rewrite, r.Enabled, r.Description)
			}
			return w.Flush()
		},
	}
}

func routeAddCmd() *cobra.Command {
	var prefix, target, rewrite, desc string
	var disabled bool

	cmd := &cobra.Command{
		Use:   "add",
		Short: "新增服务路由",
		RunE: func(cmd *cobra.Command, args []string) error {
			route := model.ServiceRoute{
				Prefix:      prefix,
				Target:      target,
				Rewrite:     rewrite,
				Description: desc,
				Enabled:     !disabled,
			}
			if err := db.Create(&route).Error; err != nil {
				return err
			}
			fmt.Printf("已创建路由: ID=%d  %s → %s\n", route.ID, prefix, target)
			return nil
		},
	}
	cmd.Flags().StringVarP(&prefix, "prefix", "p", "", "路径前缀（必填），如 /rbac")
	cmd.Flags().StringVarP(&target, "target", "t", "", "后端服务地址（必填），如 http://192.168.30.198:8000")
	cmd.Flags().StringVarP(&rewrite, "rewrite", "r", "", "路径改写前缀，如 /api")
	cmd.Flags().StringVarP(&desc, "desc", "d", "", "路由描述")
	cmd.Flags().BoolVar(&disabled, "disabled", false, "创建时禁用该路由")
	_ = cmd.MarkFlagRequired("prefix")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

func routeUpdateCmd() *cobra.Command {
	var prefix, target, rewrite, desc string
	var enable, disable bool

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "更新服务路由",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("无效的路由 ID: %s", args[0])
			}
			var route model.ServiceRoute
			if err := db.First(&route, id).Error; err != nil {
				return fmt.Errorf("路由 %d 不存在", id)
			}
			updates := map[string]any{}
			if cmd.Flags().Changed("prefix") {
				updates["prefix"] = prefix
			}
			if cmd.Flags().Changed("target") {
				updates["target"] = target
			}
			if cmd.Flags().Changed("rewrite") {
				updates["rewrite"] = rewrite
			}
			if cmd.Flags().Changed("desc") {
				updates["description"] = desc
			}
			if enable {
				updates["enabled"] = true
			} else if disable {
				updates["enabled"] = false
			}
			if len(updates) == 0 {
				return fmt.Errorf("请至少指定一个要更新的字段")
			}
			if err := db.Model(&route).Updates(updates).Error; err != nil {
				return err
			}
			fmt.Printf("路由 %d 已更新\n", id)
			return nil
		},
	}
	cmd.Flags().StringVarP(&prefix, "prefix", "p", "", "新的路径前缀")
	cmd.Flags().StringVarP(&target, "target", "t", "", "新的后端服务地址")
	cmd.Flags().StringVarP(&rewrite, "rewrite", "r", "", "新的路径改写前缀")
	cmd.Flags().StringVarP(&desc, "desc", "d", "", "新的路由描述")
	cmd.Flags().BoolVar(&enable, "enable", false, "启用路由")
	cmd.Flags().BoolVar(&disable, "disable", false, "禁用路由")
	return cmd
}

func routeDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "删除服务路由",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("无效的路由 ID: %s", args[0])
			}
			if err := db.Delete(&model.ServiceRoute{}, id).Error; err != nil {
				return err
			}
			fmt.Printf("路由 %d 已删除\n", id)
			return nil
		},
	}
}

func routeEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <id>",
		Short: "启用服务路由",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return setRouteEnabled(args[0], true) },
	}
}

func routeDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <id>",
		Short: "禁用服务路由",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return setRouteEnabled(args[0], false) },
	}
}

func setRouteEnabled(idStr string, enabled bool) error {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return fmt.Errorf("无效的路由 ID: %s", idStr)
	}
	if err := db.Model(&model.ServiceRoute{}).Where("id = ?", id).Update("enabled", enabled).Error; err != nil {
		return err
	}
	state := "启用"
	if !enabled {
		state = "禁用"
	}
	fmt.Printf("路由 %d 已%s\n", id, state)
	return nil
}

// ─── whitelist ─────────────────────────────────────────────────────────────────

func buildWhitelistCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "whitelist",
		Short: "管理白名单条目",
	}
	cmd.AddCommand(
		whitelistListCmd(),
		whitelistAddCmd(),
		whitelistDeleteCmd(),
	)
	return cmd
}

func whitelistListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出所有白名单条目",
		RunE: func(cmd *cobra.Command, args []string) error {
			var items []model.WhitelistRoute
			if err := db.Find(&items).Error; err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "ID\tMETHOD\tPATH\tDESCRIPTION")
			for _, item := range items {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", item.ID, item.Method, item.Path, item.Description)
			}
			return w.Flush()
		},
	}
}

func whitelistAddCmd() *cobra.Command {
	var method, path, desc string

	cmd := &cobra.Command{
		Use:   "add",
		Short: "新增白名单条目",
		RunE: func(cmd *cobra.Command, args []string) error {
			item := model.WhitelistRoute{
				Method:      method,
				Path:        path,
				Description: desc,
			}
			if err := db.Create(&item).Error; err != nil {
				return err
			}
			fmt.Printf("已创建白名单: ID=%d  %s %s\n", item.ID, method, path)
			return nil
		},
	}
	cmd.Flags().StringVarP(&method, "method", "m", "*", "HTTP 方法，* 表示任意方法")
	cmd.Flags().StringVarP(&path, "path", "p", "", "请求路径，支持前缀通配（以 /* 结尾）")
	cmd.Flags().StringVarP(&desc, "desc", "d", "", "描述")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func whitelistDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "删除白名单条目",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("无效的白名单 ID: %s", args[0])
			}
			if err := db.Delete(&model.WhitelistRoute{}, id).Error; err != nil {
				return err
			}
			fmt.Printf("白名单 %d 已删除\n", id)
			return nil
		},
	}
}
