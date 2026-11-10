<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="utf-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>最新更改记录</title>
    <style>
        * { box-sizing: border-box; }

        body {
            margin: 0;
            padding: 10px;
            background: #ffffff;
            color: #333;
            font-size: 13px;
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", Arial, sans-serif;
        }

        .changelog-meta {
            color: #999;
            font-size: 12px;
            margin: 0 2px 8px;
        }

        table.changelog {
            width: 100%;
            border-collapse: collapse;
            table-layout: auto;
        }

        table.changelog th {
            background: #f7f7f7;
            color: #666;
            font-weight: 600;
            text-align: left;
            padding: 9px 12px;
            border-bottom: 1px solid #e8e8e8;
            white-space: nowrap;
        }

        table.changelog td {
            padding: 9px 12px;
            border-bottom: 1px solid #f0f0f0;
            vertical-align: middle;
        }

        table.changelog tbody tr:nth-child(even) {
            background: #fafafa;
        }

        table.changelog tbody tr:hover {
            background: #f3f8ff;
        }

        table.changelog td.title {
            max-width: 420px;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }

        table.changelog a {
            color: #1677ff;
            text-decoration: none;
        }

        table.changelog a:hover {
            text-decoration: underline;
        }

        .changelog-time {
            color: #666;
            white-space: nowrap;
        }

        .changelog-empty {
            color: #999;
            text-align: center;
            padding: 48px 0;
        }
    </style>
</head>
<body>
<div class="changelog-meta">{{.RangeLabel}}修改的文档，共 {{len .Changes}} 篇</div>
{{if .Changes}}
<table class="changelog">
    <thead>
    <tr>
        <th>标题</th>
        <th>栏目</th>
        <th>修改时间</th>
        <th>修改人</th>
        <th>创建时间</th>
        <th>创建人</th>
    </tr>
    </thead>
    <tbody>
    {{range .Changes}}
    <tr>
        <td class="title"><a href="{{.URL}}" target="_blank" title="{{.DocumentName}}">{{.DocumentName}}</a></td>
        <td>{{.BookName}}</td>
        <td class="changelog-time">{{.ModifyTimeText}}</td>
        <td>{{.ModifyBy}}</td>
        <td class="changelog-time">{{.CreateTimeText}}</td>
        <td>{{.CreateBy}}</td>
    </tr>
    {{end}}
    </tbody>
</table>
{{else}}
<div class="changelog-empty">{{.RangeLabel}}没有文档更新</div>
{{end}}
</body>
</html>
